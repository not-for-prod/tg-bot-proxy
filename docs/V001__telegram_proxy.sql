-- V001__telegram_proxy.sql
-- PostgreSQL 15+
--
-- Application configuration supplies bot IDs, bot tokens, and supergroup ID.

BEGIN;

CREATE TABLE telegram_bot_offset (
    bot_id          bigint PRIMARY KEY,
    update_id       bigint NOT NULL DEFAULT 0
                        CHECK (update_id >= 0),

    lease_owner     uuid,
    lease_until     timestamptz,
    lease_version   bigint NOT NULL DEFAULT 0
                        CHECK (lease_version >= 0),

    CONSTRAINT telegram_bot_offset_lease_chk
        CHECK (
            (lease_owner IS NULL AND lease_until IS NULL)
            OR
            (lease_owner IS NOT NULL AND lease_until IS NOT NULL)
        )
);

COMMENT ON TABLE telegram_bot_offset IS
'Next getUpdates offset and fenced polling lease for one configured Telegram bot.';

COMMENT ON COLUMN telegram_bot_offset.update_id IS
'Next offset passed to getUpdates. A missing row is treated as offset 0.';

COMMENT ON COLUMN telegram_bot_offset.lease_version IS
'Monotonically increasing fencing token for polling ownership.';


CREATE TABLE inbox_processor_lock (
    bot_id          bigint PRIMARY KEY,

    lease_owner     uuid,
    lease_until     timestamptz,
    lease_version   bigint NOT NULL DEFAULT 0
                        CHECK (lease_version >= 0),

    CONSTRAINT inbox_processor_lock_lease_chk
        CHECK (
            (lease_owner IS NULL AND lease_until IS NULL)
            OR
            (lease_owner IS NOT NULL AND lease_until IS NOT NULL)
        )
);

COMMENT ON TABLE inbox_processor_lock IS
'Fenced lease ensuring one ordered inbox processor for the configured bot stream.';


CREATE TABLE chats (
    id                       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    external_chat_id         bigint NOT NULL UNIQUE,
    supergroup_id            bigint NOT NULL,
    topic_id                 bigint,

    next_chat_offset         bigint NOT NULL DEFAULT 1
                                 CHECK (next_chat_offset >= 1),

    committed_chat_offset    bigint NOT NULL DEFAULT 0
                                 CHECK (committed_chat_offset >= 0),

    created_at               timestamptz NOT NULL DEFAULT clock_timestamp(),
    last_activity_at         timestamptz NOT NULL DEFAULT clock_timestamp(),

    CONSTRAINT chats_offset_range_chk
        CHECK (committed_chat_offset < next_chat_offset)
);

CREATE UNIQUE INDEX chats_topic_uq
    ON chats (supergroup_id, topic_id)
    WHERE topic_id IS NOT NULL;

CREATE INDEX chats_last_activity_idx
    ON chats (last_activity_at);

COMMENT ON TABLE chats IS
'External private-chat to internal forum-topic mapping and outbox partition state.';

COMMENT ON COLUMN chats.topic_id IS
'Current Telegram message_thread_id; NULL until a topic is created.';

COMMENT ON COLUMN chats.next_chat_offset IS
'Next outbox position allocated inside this chat partition.';

COMMENT ON COLUMN chats.committed_chat_offset IS
'Highest successfully completed outbox position inside this chat partition.';


CREATE TABLE inbox (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    bot_id              bigint NOT NULL,
    update_id           bigint NOT NULL,
    payload             jsonb NOT NULL,

    status              text NOT NULL DEFAULT 'pending'
                            CHECK (status IN (
                                'pending',
                                'retry',
                                'failed'
                            )),

    attempt_count       integer NOT NULL DEFAULT 0
                            CHECK (attempt_count >= 0),

    available_at        timestamptz NOT NULL DEFAULT clock_timestamp(),

    last_error          text,
    failed_at           timestamptz,

    created_at          timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at          timestamptz NOT NULL DEFAULT clock_timestamp(),

    CONSTRAINT inbox_bot_update_uq
        UNIQUE (bot_id, update_id),

    CONSTRAINT inbox_payload_update_id_chk
        CHECK (
            payload ? 'update_id'
            AND jsonb_typeof(payload -> 'update_id') = 'number'
            AND (payload ->> 'update_id')::bigint = update_id
        ),

    CONSTRAINT inbox_failed_at_chk
        CHECK (
            (status = 'failed' AND failed_at IS NOT NULL)
            OR
            (status <> 'failed' AND failed_at IS NULL)
        )
);

CREATE INDEX inbox_ready_idx
    ON inbox (bot_id, available_at, update_id)
    WHERE status IN ('pending', 'retry');

CREATE INDEX inbox_failed_idx
    ON inbox (bot_id, update_id)
    WHERE status = 'failed';

COMMENT ON TABLE inbox IS
'Raw Telegram updates awaiting processing. Successful and ignored rows are deleted.';


CREATE TABLE outbox (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    chat_id               bigint NOT NULL
                              REFERENCES chats(id),

    chat_offset           bigint NOT NULL
                              CHECK (chat_offset >= 1),

    deduplication_key     text NOT NULL UNIQUE,

    operation             text NOT NULL
                              CHECK (operation IN (
                                  'relay_external_to_internal',
                                  'relay_internal_to_external',
                                  'send_external_message'
                              )),

    payload               jsonb NOT NULL,

    status                text NOT NULL DEFAULT 'pending'
                              CHECK (status IN (
                                  'pending',
                                  'processing',
                                  'retry',
                                  'failed'
                              )),

    attempt_count         integer NOT NULL DEFAULT 0
                              CHECK (attempt_count >= 0),

    available_at          timestamptz NOT NULL DEFAULT clock_timestamp(),

    lease_owner           uuid,
    lease_until           timestamptz,
    lease_version         bigint NOT NULL DEFAULT 0
                              CHECK (lease_version >= 0),

    last_http_status      integer,
    last_error_code       integer,
    last_error            text,

    retry_after_seconds   integer
                              CHECK (
                                  retry_after_seconds IS NULL
                                  OR retry_after_seconds >= 0
                              ),

    failed_at             timestamptz,

    created_at            timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at            timestamptz NOT NULL DEFAULT clock_timestamp(),

    CONSTRAINT outbox_chat_offset_uq
        UNIQUE (chat_id, chat_offset),

    CONSTRAINT outbox_processing_lease_chk
        CHECK (
            (
                status = 'processing'
                AND lease_owner IS NOT NULL
                AND lease_until IS NOT NULL
            )
            OR
            (
                status <> 'processing'
                AND lease_owner IS NULL
                AND lease_until IS NULL
            )
        ),

    CONSTRAINT outbox_failed_at_chk
        CHECK (
            (status = 'failed' AND failed_at IS NOT NULL)
            OR
            (status <> 'failed' AND failed_at IS NULL)
        )
);

CREATE INDEX outbox_ready_idx
    ON outbox (available_at, chat_id, chat_offset)
    WHERE status IN ('pending', 'retry');

CREATE UNIQUE INDEX outbox_one_processing_per_chat_uq
    ON outbox (chat_id)
    WHERE status = 'processing';

CREATE INDEX outbox_expired_lease_idx
    ON outbox (lease_until)
    WHERE status = 'processing';

CREATE INDEX outbox_failed_idx
    ON outbox (chat_id, chat_offset)
    WHERE status = 'failed';

COMMENT ON TABLE outbox IS
'Durable high-level proxy operations ordered by chat partition and chat offset.';

COMMENT ON COLUMN outbox.chat_id IS
'Outbox partition key.';

COMMENT ON COLUMN outbox.chat_offset IS
'Ordered position inside the chat partition.';

COMMENT ON COLUMN outbox.operation IS
'Application operation rather than a raw Telegram Bot API method.';

COMMENT ON COLUMN outbox.payload IS
'Normalized operation-specific data required to execute or retry the operation.';

COMMENT ON COLUMN outbox.lease_version IS
'Monotonically increasing fencing token for publisher ownership.';

COMMIT;
