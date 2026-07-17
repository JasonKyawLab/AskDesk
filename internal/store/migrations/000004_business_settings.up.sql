-- Per-business, runtime-editable settings (shop name, welcome/fallback/ask
-- messages). JSONB so new settings never need another schema change.
ALTER TABLE businesses ADD COLUMN settings JSONB NOT NULL DEFAULT '{}';
