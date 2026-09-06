-- Multi-language FAQs: each FAQ belongs to one language so a business can write
-- native-tone answers per language (no machine translation). RAG retrieval and
-- the widget filter by language. Existing rows default to English.
ALTER TABLE faqs ADD COLUMN language TEXT NOT NULL DEFAULT 'en';
CREATE INDEX faqs_business_language_idx ON faqs (business_id, language);
