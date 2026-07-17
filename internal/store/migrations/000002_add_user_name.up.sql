-- Store the customer's display name so admins can see who asked an unanswered
-- question (the external_user_id alone is just a number).
ALTER TABLE conversations ADD COLUMN external_user_name TEXT;
