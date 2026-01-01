-- Migration: Create task_completions table
-- Version: 003

CREATE TABLE IF NOT EXISTS task_completions (
    id SERIAL PRIMARY KEY,
    student_id UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    task_id VARCHAR(100) NOT NULL,
    task_slug VARCHAR(200) NOT NULL,
    bootcamp_id VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'completed',
    completed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    UNIQUE(student_id, task_id)
);

CREATE INDEX idx_task_completions_student_id ON task_completions(student_id);
CREATE INDEX idx_task_completions_bootcamp_id ON task_completions(bootcamp_id);
CREATE INDEX idx_task_completions_completed_at ON task_completions(completed_at DESC);

-- Apply trigger to task_completions table
CREATE TRIGGER update_task_completions_updated_at
    BEFORE UPDATE ON task_completions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
