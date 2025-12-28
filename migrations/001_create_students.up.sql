-- Migration: Create students table
-- Version: 001

CREATE TABLE IF NOT EXISTS students (
    id UUID PRIMARY KEY,
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
    
    -- Notification preferences (JSON)
    preferences JSONB NOT NULL DEFAULT '{
        "rank_changes": true,
        "daily_digest": true,
        "help_requests": true,
        "inactivity_reminders": true,
        "quiet_hours_start": 23,
        "quiet_hours_end": 8
    }'::jsonb
);

-- Indexes for common queries
CREATE INDEX idx_students_telegram_id ON students(telegram_id);
CREATE INDEX idx_students_alem_login ON students(alem_login);
CREATE INDEX idx_students_cohort ON students(cohort);
CREATE INDEX idx_students_status ON students(status);
CREATE INDEX idx_students_current_xp ON students(current_xp DESC);
CREATE INDEX idx_students_last_seen_at ON students(last_seen_at);
CREATE INDEX idx_students_online_state ON students(online_state) WHERE online_state != 'offline';

-- XP History table for tracking changes
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

CREATE INDEX idx_xp_history_student_id ON xp_history(student_id);
CREATE INDEX idx_xp_history_created_at ON xp_history(created_at);

-- Daily grind table for daily progress tracking
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

CREATE INDEX idx_daily_grinds_student_date ON daily_grinds(student_id, date DESC);

-- Streaks table
CREATE TABLE IF NOT EXISTS streaks (
    student_id UUID PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
    current_streak INTEGER NOT NULL DEFAULT 0,
    best_streak INTEGER NOT NULL DEFAULT 0,
    last_active_date DATE,
    streak_start_date DATE
);

-- Achievements table
CREATE TABLE IF NOT EXISTS achievements (
    id SERIAL PRIMARY KEY,
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    achievement_type VARCHAR(50) NOT NULL,
    unlocked_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    metadata JSONB,
    
    UNIQUE(student_id, achievement_type)
);

CREATE INDEX idx_achievements_student_id ON achievements(student_id);
CREATE INDEX idx_achievements_unlocked_at ON achievements(unlocked_at DESC);

-- Sync errors table for debugging
CREATE TABLE IF NOT EXISTS sync_errors (
    id SERIAL PRIMARY KEY,
    student_id UUID REFERENCES students(id) ON DELETE SET NULL,
    error_type VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    occurred_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    retries INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_sync_errors_occurred_at ON sync_errors(occurred_at DESC);

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to students table
CREATE TRIGGER update_students_updated_at
    BEFORE UPDATE ON students
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
