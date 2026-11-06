INSERT INTO inbox (
    bot_id,
    update_id,
    payload
)
SELECT $1, updates.update_id, updates.payload
FROM unnest($2::bigint[], $3::jsonb[]) AS updates(update_id, payload)
ON CONFLICT (bot_id, update_id) DO NOTHING;
