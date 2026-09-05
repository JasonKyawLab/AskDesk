-- Leads: contact details captured from web-widget visitors (the contact-gate).
-- One row per (business, widget session); upserted as the visitor provides info.
CREATE TABLE leads (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_id BIGINT      NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    session_id  TEXT        NOT NULL,
    name        TEXT,
    email       TEXT,
    phone       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, session_id)
);

CREATE INDEX leads_business_id_idx ON leads (business_id, created_at DESC);
