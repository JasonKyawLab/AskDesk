-- Tracks which unanswered-queue item an admin is currently replying to (the
-- tap-to-reply flow: tap Reply on a question, then type the answer).
ALTER TABLE admins ADD COLUMN pending_reply_id BIGINT;
