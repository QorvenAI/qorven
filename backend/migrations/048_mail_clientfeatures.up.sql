-- 048_mail_clientfeatures: connection transport + client-grade per-identity prefs
-- + message importance + search/folder indexes.
ALTER TABLE soul_mail_identities
  ADD COLUMN IF NOT EXISTS transport          text NOT NULL DEFAULT 'native',
  ADD COLUMN IF NOT EXISTS forward_url         text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS inbound_secret_enc  text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS signature_html      text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS signature_text      text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS reply_to            text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS default_importance  text NOT NULL DEFAULT 'normal';
ALTER TABLE mailbox_messages
  ADD COLUMN IF NOT EXISTS importance text NOT NULL DEFAULT 'normal';
CREATE INDEX IF NOT EXISTS idx_mailbox_folder ON mailbox_messages (tenant_id, folder, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_mailbox_search ON mailbox_messages USING gin (to_tsvector('english', coalesce(subject,'') || ' ' || coalesce(body_text,'')));
