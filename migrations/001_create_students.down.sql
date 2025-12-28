-- Rollback migration: Drop students table and related tables

DROP TRIGGER IF EXISTS update_students_updated_at ON students;
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS sync_errors;
DROP TABLE IF EXISTS achievements;
DROP TABLE IF EXISTS streaks;
DROP TABLE IF EXISTS daily_grinds;
DROP TABLE IF EXISTS xp_history;
DROP TABLE IF EXISTS students;
