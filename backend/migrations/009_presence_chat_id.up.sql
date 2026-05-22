-- Migration 009: add chat_id to user_presence for offline channel nudges
ALTER TABLE user_presence ADD COLUMN IF NOT EXISTS chat_id text NOT NULL DEFAULT '';
