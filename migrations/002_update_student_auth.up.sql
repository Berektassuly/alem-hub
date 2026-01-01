-- Migration: Update student auth (remove alem_login, add email/password)
-- Version: 002

-- Alter table to add new columns first
ALTER TABLE students ADD COLUMN IF NOT EXISTS email VARCHAR(255);
ALTER TABLE students ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255);

-- Update existing records if any (optional, but good practice to avoid not-null errors if data exists)
-- Since we are "deleting" alem_login and user implies this is a breaking change/new start, we can just truncate or ignore old data validity for now,
-- but to be safe with NOT NULL constraints:
UPDATE students SET email = CONCAT(id, '@placeholder.com'), password_hash = 'placeholder' WHERE email IS NULL;

-- Now add constraints
ALTER TABLE students ALTER COLUMN email SET NOT NULL;
ALTER TABLE students ALTER COLUMN password_hash SET NOT NULL;

ALTER TABLE students ADD CONSTRAINT students_email_key UNIQUE (email);

-- Drop alem_login
ALTER TABLE students DROP COLUMN IF EXISTS alem_login;

-- Update indexes
DROP INDEX IF EXISTS idx_students_alem_login;
CREATE INDEX IF NOT EXISTS idx_students_email ON students(email);
