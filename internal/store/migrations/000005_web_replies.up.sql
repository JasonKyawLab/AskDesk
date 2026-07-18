-- Replies to web/widget customers wait here until the customer's browser polls
-- for them (a web tab can't be pushed to like a Telegram chat).
CREATE TABLE web_replies (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    business_id BIGINT      NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    session_id  TEXT        NOT NULL,
    message     TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX web_replies_lookup_idx ON web_replies (business_id, session_id, id);
