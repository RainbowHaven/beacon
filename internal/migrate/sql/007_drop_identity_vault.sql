-- Phase: remove sealed identity vault (legal name / UN ID ciphertext).

ALTER TABLE occupants DROP CONSTRAINT IF EXISTS occupants_ciphertext_chk;
ALTER TABLE occupants DROP COLUMN IF EXISTS identity_ciphertext;
ALTER TABLE occupants DROP COLUMN IF EXISTS key_id;
