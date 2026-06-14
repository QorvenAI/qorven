-- Record who installed each app and from where (install provenance).
ALTER TABLE apps ADD COLUMN IF NOT EXISTS installed_by uuid;
ALTER TABLE apps ADD COLUMN IF NOT EXISTS source_path text;
