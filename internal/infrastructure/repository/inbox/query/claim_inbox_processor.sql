INSERT INTO inbox_processor_lock (
    bot_id,
    lease_owner,
    lease_until,
    lease_version
)
VALUES ($1, $2, $3, 1)
ON CONFLICT (bot_id) DO UPDATE
SET lease_owner = EXCLUDED.lease_owner,
    lease_until = EXCLUDED.lease_until,
    lease_version = inbox_processor_lock.lease_version + 1
WHERE inbox_processor_lock.lease_owner IS NULL
   OR inbox_processor_lock.lease_until <= clock_timestamp()
RETURNING lease_version;
