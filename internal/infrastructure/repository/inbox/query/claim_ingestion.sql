INSERT INTO telegram_bot_offset (
    bot_id,
    lease_owner,
    lease_until,
    lease_version
)
VALUES ($1, $2, $3, 1)
ON CONFLICT (bot_id) DO UPDATE
SET lease_owner = EXCLUDED.lease_owner,
    lease_until = EXCLUDED.lease_until,
    lease_version = telegram_bot_offset.lease_version + 1
WHERE telegram_bot_offset.lease_owner IS NULL
   OR telegram_bot_offset.lease_until <= clock_timestamp()
RETURNING update_id, lease_version;
