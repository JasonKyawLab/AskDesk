-- Initial schema. Every table is scoped by business_id so multi-tenancy is a
-- new row, not a migration.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE businesses (
    id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name                   TEXT        NOT NULL,
    api_key                TEXT        NOT NULL UNIQUE,
    telegram_bot_token_enc BYTEA,        -- encrypted at rest, never plaintext
    whatsapp_number        TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE faqs (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_id BIGINT      NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    question    TEXT        NOT NULL,
    answer      TEXT        NOT NULL,
    embedding   vector(768),
    category    TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX faqs_business_id_idx ON faqs (business_id);
-- Approximate nearest-neighbour index for RAG similarity search (cosine).
CREATE INDEX faqs_embedding_idx ON faqs USING hnsw (embedding vector_cosine_ops);

CREATE TABLE conversations (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_id      BIGINT      NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    channel          TEXT        NOT NULL,
    external_user_id TEXT        NOT NULL,
    question         TEXT        NOT NULL,
    matched_faq_id   BIGINT      REFERENCES faqs(id) ON DELETE SET NULL,
    ai_answer        TEXT,
    confidence_score REAL,
    was_answered     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX conversations_business_id_idx ON conversations (business_id, created_at DESC);

CREATE TABLE unanswered_queue (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id BIGINT      NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    question        TEXT        NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'pending', -- pending | resolved
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX unanswered_queue_status_idx ON unanswered_queue (status);

CREATE TABLE admins (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_id BIGINT NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    channel     TEXT   NOT NULL,
    external_id TEXT   NOT NULL, -- Telegram user_id, WhatsApp number, or minipos user id
    name        TEXT,
    UNIQUE (business_id, channel, external_id)
);
