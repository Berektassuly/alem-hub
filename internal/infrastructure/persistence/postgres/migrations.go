// Package postgres implements PostgreSQL persistence layer for Alem Community Hub.
package postgres

// ══════════════════════════════════════════════════════════════════════════════
// MIGRATION 001: CREATE STUDENTS
// ══════════════════════════════════════════════════════════════════════════════

const migration001Up = `
-- Migration: Create students table
-- Version: 001
-- Philosophy: "From Competition to Collaboration"

-- Main students table
CREATE TABLE IF NOT EXISTS students (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_id BIGINT NOT NULL UNIQUE,
    alem_login VARCHAR(50) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    current_xp INTEGER NOT NULL DEFAULT 0,
    cohort VARCHAR(30) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    online_state VARCHAR(20) NOT NULL DEFAULT 'offline',
    last_seen_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_synced_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    joined_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    help_rating DECIMAL(3,2) NOT NULL DEFAULT 0.00,
    help_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Notification preferences (JSONB for flexibility)
    preferences JSONB NOT NULL DEFAULT '{
        "rank_changes": true,
        "daily_digest": true,
        "help_requests": true,
        "inactivity_reminders": true,
        "quiet_hours_start": 23,
        "quiet_hours_end": 8
    }'::jsonb,

    -- Constraints for data integrity
    CONSTRAINT valid_status CHECK (status IN ('active', 'inactive', 'graduated', 'left', 'suspended')),
    CONSTRAINT valid_online_state CHECK (online_state IN ('online', 'away', 'offline')),
    CONSTRAINT valid_xp CHECK (current_xp >= 0),
    CONSTRAINT valid_help_rating CHECK (help_rating >= 0 AND help_rating <= 5)
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_students_telegram_id ON students(telegram_id);
CREATE INDEX IF NOT EXISTS idx_students_alem_login ON students(alem_login);
CREATE INDEX IF NOT EXISTS idx_students_cohort ON students(cohort);
CREATE INDEX IF NOT EXISTS idx_students_status ON students(status);
CREATE INDEX IF NOT EXISTS idx_students_current_xp ON students(current_xp DESC);
CREATE INDEX IF NOT EXISTS idx_students_last_seen_at ON students(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_students_online_state ON students(online_state) WHERE online_state != 'offline';

-- Composite index for leaderboard queries
CREATE INDEX IF NOT EXISTS idx_students_cohort_xp ON students(cohort, current_xp DESC);
CREATE INDEX IF NOT EXISTS idx_students_active_xp ON students(current_xp DESC) WHERE status = 'active';

-- XP History table for tracking changes over time
CREATE TABLE IF NOT EXISTS xp_history (
    id SERIAL PRIMARY KEY,
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    old_xp INTEGER NOT NULL,
    new_xp INTEGER NOT NULL,
    delta INTEGER NOT NULL,
    reason VARCHAR(50) NOT NULL,
    task_id VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_xp_history_student_id ON xp_history(student_id);
CREATE INDEX IF NOT EXISTS idx_xp_history_created_at ON xp_history(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_xp_history_student_date ON xp_history(student_id, created_at DESC);

-- Daily grind table for daily progress tracking ("Daily Grind" feature)
CREATE TABLE IF NOT EXISTS daily_grinds (
    id SERIAL PRIMARY KEY,
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    xp_start INTEGER NOT NULL,
    xp_current INTEGER NOT NULL,
    xp_gained INTEGER NOT NULL DEFAULT 0,
    tasks_completed INTEGER NOT NULL DEFAULT 0,
    sessions_count INTEGER NOT NULL DEFAULT 0,
    total_session_minutes INTEGER NOT NULL DEFAULT 0,
    first_activity_at TIMESTAMP WITH TIME ZONE,
    last_activity_at TIMESTAMP WITH TIME ZONE,
    rank_at_start INTEGER NOT NULL DEFAULT 0,
    rank_current INTEGER NOT NULL DEFAULT 0,
    rank_change INTEGER NOT NULL DEFAULT 0,
    streak_day INTEGER NOT NULL DEFAULT 0,
    
    UNIQUE(student_id, date)
);

CREATE INDEX IF NOT EXISTS idx_daily_grinds_student_date ON daily_grinds(student_id, date DESC);

-- Streaks table for tracking consecutive active days
CREATE TABLE IF NOT EXISTS streaks (
    student_id UUID PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
    current_streak INTEGER NOT NULL DEFAULT 0,
    best_streak INTEGER NOT NULL DEFAULT 0,
    last_active_date DATE,
    streak_start_date DATE
);

-- Achievements table for gamification
CREATE TABLE IF NOT EXISTS achievements (
    id SERIAL PRIMARY KEY,
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    achievement_type VARCHAR(50) NOT NULL,
    unlocked_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    metadata JSONB,
    
    UNIQUE(student_id, achievement_type)
);

CREATE INDEX IF NOT EXISTS idx_achievements_student_id ON achievements(student_id);
CREATE INDEX IF NOT EXISTS idx_achievements_unlocked_at ON achievements(unlocked_at DESC);
CREATE INDEX IF NOT EXISTS idx_achievements_type ON achievements(achievement_type);

-- Sync errors table for debugging and monitoring
CREATE TABLE IF NOT EXISTS sync_errors (
    id SERIAL PRIMARY KEY,
    student_id UUID REFERENCES students(id) ON DELETE SET NULL,
    error_type VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    occurred_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    retries INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sync_errors_occurred_at ON sync_errors(occurred_at DESC);

-- Updated_at trigger function for automatic timestamp updates
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to students table
DROP TRIGGER IF EXISTS update_students_updated_at ON students;
CREATE TRIGGER update_students_updated_at
    BEFORE UPDATE ON students
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
`

const migration001Down = `
DROP TRIGGER IF EXISTS update_students_updated_at ON students;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP TABLE IF EXISTS sync_errors;
DROP TABLE IF EXISTS achievements;
DROP TABLE IF EXISTS streaks;
DROP TABLE IF EXISTS daily_grinds;
DROP TABLE IF EXISTS xp_history;
DROP TABLE IF EXISTS students;
`

// ══════════════════════════════════════════════════════════════════════════════
// MIGRATION 002: CREATE LEADERBOARD
// ══════════════════════════════════════════════════════════════════════════════

const migration002Up = `
-- Migration: Create leaderboard tables
-- Version: 002
-- Purpose: Track rankings and historical leaderboard data

-- Leaderboard snapshots for historical tracking
CREATE TABLE IF NOT EXISTS leaderboard_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cohort VARCHAR(30) NOT NULL DEFAULT '',
    snapshot_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    total_students INTEGER NOT NULL DEFAULT 0,
    total_xp BIGINT NOT NULL DEFAULT 0,
    average_xp INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_leaderboard_snapshots_cohort ON leaderboard_snapshots(cohort);
CREATE INDEX IF NOT EXISTS idx_leaderboard_snapshots_at ON leaderboard_snapshots(snapshot_at DESC);
CREATE INDEX IF NOT EXISTS idx_leaderboard_snapshots_cohort_at ON leaderboard_snapshots(cohort, snapshot_at DESC);

-- Leaderboard entries for each snapshot
CREATE TABLE IF NOT EXISTS leaderboard_entries (
    id SERIAL PRIMARY KEY,
    snapshot_id UUID NOT NULL REFERENCES leaderboard_snapshots(id) ON DELETE CASCADE,
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    rank INTEGER NOT NULL,
    xp INTEGER NOT NULL,
    level INTEGER NOT NULL DEFAULT 0,
    rank_change INTEGER NOT NULL DEFAULT 0,
    is_online BOOLEAN NOT NULL DEFAULT FALSE,
    is_available_for_help BOOLEAN NOT NULL DEFAULT FALSE,
    
    UNIQUE(snapshot_id, student_id)
);

CREATE INDEX IF NOT EXISTS idx_leaderboard_entries_snapshot ON leaderboard_entries(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_leaderboard_entries_student ON leaderboard_entries(student_id);
CREATE INDEX IF NOT EXISTS idx_leaderboard_entries_rank ON leaderboard_entries(snapshot_id, rank);

-- Rank history for individual tracking over time
CREATE TABLE IF NOT EXISTS rank_history (
    id SERIAL PRIMARY KEY,
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    rank INTEGER NOT NULL,
    xp INTEGER NOT NULL,
    snapshot_id UUID NOT NULL REFERENCES leaderboard_snapshots(id) ON DELETE CASCADE,
    snapshot_at TIMESTAMP WITH TIME ZONE NOT NULL,
    rank_change INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_rank_history_student ON rank_history(student_id);
CREATE INDEX IF NOT EXISTS idx_rank_history_student_at ON rank_history(student_id, snapshot_at DESC);
`

const migration002Down = `
DROP TABLE IF EXISTS rank_history;
DROP TABLE IF EXISTS leaderboard_entries;
DROP TABLE IF EXISTS leaderboard_snapshots;
`

// ══════════════════════════════════════════════════════════════════════════════
// MIGRATION 003: CREATE SOCIAL
// ══════════════════════════════════════════════════════════════════════════════

const migration003Up = `
-- Migration: Create social tables
-- Version: 003
-- Philosophy: Enable collaboration and peer support

-- Connections between students (study buddies, mentor relationships)
CREATE TABLE IF NOT EXISTS connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    to_student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    connection_type VARCHAR(30) NOT NULL DEFAULT 'peer',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    UNIQUE(from_student_id, to_student_id),
    CONSTRAINT different_students CHECK (from_student_id != to_student_id),
    CONSTRAINT valid_connection_type CHECK (connection_type IN ('peer', 'mentor', 'study_buddy'))
);

CREATE INDEX IF NOT EXISTS idx_connections_from ON connections(from_student_id);
CREATE INDEX IF NOT EXISTS idx_connections_to ON connections(to_student_id);

-- Help requests - core feature for community support
CREATE TABLE IF NOT EXISTS help_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requester_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    task_id VARCHAR(100) NOT NULL,
    task_name VARCHAR(255),
    message TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'open',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMP WITH TIME ZONE,
    helper_id UUID REFERENCES students(id) ON DELETE SET NULL,
    
    CONSTRAINT valid_help_status CHECK (status IN ('open', 'in_progress', 'resolved', 'cancelled'))
);

CREATE INDEX IF NOT EXISTS idx_help_requests_requester ON help_requests(requester_id);
CREATE INDEX IF NOT EXISTS idx_help_requests_task ON help_requests(task_id);
CREATE INDEX IF NOT EXISTS idx_help_requests_status ON help_requests(status) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS idx_help_requests_helper ON help_requests(helper_id) WHERE helper_id IS NOT NULL;

-- Endorsements (thank you for help) - builds reputation
CREATE TABLE IF NOT EXISTS endorsements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    to_student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    help_request_id UUID REFERENCES help_requests(id) ON DELETE SET NULL,
    rating INTEGER NOT NULL DEFAULT 5,
    message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    CONSTRAINT valid_rating CHECK (rating >= 1 AND rating <= 5),
    CONSTRAINT different_students_endorse CHECK (from_student_id != to_student_id)
);

CREATE INDEX IF NOT EXISTS idx_endorsements_from ON endorsements(from_student_id);
CREATE INDEX IF NOT EXISTS idx_endorsements_to ON endorsements(to_student_id);
CREATE INDEX IF NOT EXISTS idx_endorsements_created ON endorsements(created_at DESC);

-- Task completions (who solved what) - enables finding helpers
CREATE TABLE IF NOT EXISTS task_completions (
    id SERIAL PRIMARY KEY,
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    task_id VARCHAR(100) NOT NULL,
    task_name VARCHAR(255),
    xp_earned INTEGER NOT NULL DEFAULT 0,
    completed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    UNIQUE(student_id, task_id)
);

CREATE INDEX IF NOT EXISTS idx_task_completions_student ON task_completions(student_id);
CREATE INDEX IF NOT EXISTS idx_task_completions_task ON task_completions(task_id);
CREATE INDEX IF NOT EXISTS idx_task_completions_recent ON task_completions(completed_at DESC);

-- Function to update help rating when endorsement is added
CREATE OR REPLACE FUNCTION update_help_rating()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE students
    SET 
        help_rating = (
            SELECT COALESCE(AVG(rating)::DECIMAL(3,2), 0)
            FROM endorsements
            WHERE to_student_id = NEW.to_student_id
        ),
        help_count = (
            SELECT COUNT(*)
            FROM endorsements
            WHERE to_student_id = NEW.to_student_id
        )
    WHERE id = NEW.to_student_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS update_help_rating_trigger ON endorsements;
CREATE TRIGGER update_help_rating_trigger
    AFTER INSERT ON endorsements
    FOR EACH ROW
    EXECUTE FUNCTION update_help_rating();
`

const migration003Down = `
DROP TRIGGER IF EXISTS update_help_rating_trigger ON endorsements;
DROP FUNCTION IF EXISTS update_help_rating();
DROP TABLE IF EXISTS task_completions;
DROP TABLE IF EXISTS endorsements;
DROP TABLE IF EXISTS help_requests;
DROP TABLE IF EXISTS connections;
`
