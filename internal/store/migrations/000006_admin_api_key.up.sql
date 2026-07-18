-- A separate, privileged key for the admin API (list/reply/dismiss). Kept apart
-- from the public api_key so customer-facing keys never gain admin powers.
ALTER TABLE businesses ADD COLUMN admin_api_key TEXT NOT NULL DEFAULT gen_random_uuid()::text;
