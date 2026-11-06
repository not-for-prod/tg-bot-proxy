WITH candidates AS (
    SELECT o.id
    FROM outbox AS o
    JOIN chats AS c ON c.id = o.chat_id
    WHERE (
        (o.status IN ('pending', 'retry') AND o.available_at <= $1)
        OR (o.status = 'processing' AND o.lease_until <= $1)
    )
      AND o.chat_offset = c.committed_chat_offset + 1
    ORDER BY o.available_at, o.chat_id
    LIMIT $2
    FOR UPDATE OF o SKIP LOCKED
)
UPDATE outbox AS o
SET status = 'processing',
    lease_owner = $3,
    lease_until = $4,
    lease_version = o.lease_version + 1,
    attempt_count = o.attempt_count + 1,
    updated_at = clock_timestamp()
FROM candidates AS x, chats AS c
WHERE o.id = x.id
  AND c.id = o.chat_id
RETURNING o.id,
          o.chat_id,
          o.chat_offset,
          o.operation,
          o.payload,
          o.attempt_count,
          o.lease_owner,
          o.lease_version,
          c.external_chat_id,
          c.supergroup_id,
          c.topic_id;
