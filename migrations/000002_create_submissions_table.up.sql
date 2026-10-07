-- 000002_create_submissions_table.up.sql
-- Creates the submissions table with full execution state tracking, metric telemetry,
-- foreign-key relationship to users, and index optimizations.

CREATE TABLE IF NOT EXISTS submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    language VARCHAR(32) NOT NULL,
    code TEXT NOT NULL,
    stdin TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'QUEUED',
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    compilation_output TEXT NOT NULL DEFAULT '',
    stdout_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    stderr_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    exit_code INTEGER NULL,
    execution_time_ms BIGINT NULL,
    memory_usage_kb BIGINT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_submissions_language CHECK (language IN ('python', 'go')),
    CONSTRAINT chk_submissions_code_not_empty CHECK (char_length(trim(code)) > 0),
    CONSTRAINT chk_submissions_status CHECK (
        status IN (
            'QUEUED',
            'PROCESSING',
            'SUCCESS',
            'COMPILATION_ERROR',
            'RUNTIME_ERROR',
            'TIME_LIMIT_EXCEEDED',
            'MEMORY_LIMIT_EXCEEDED',
            'SYSTEM_ERROR'
        )
    ),
    CONSTRAINT chk_submissions_retry_count_non_negative CHECK (retry_count >= 0),
    CONSTRAINT chk_submissions_execution_time_non_negative CHECK (execution_time_ms IS NULL OR execution_time_ms >= 0),
    CONSTRAINT chk_submissions_memory_usage_non_negative CHECK (memory_usage_kb IS NULL OR memory_usage_kb >= 0)
);

-- Optimize user submission history queries (ordered by newest first)
CREATE INDEX IF NOT EXISTS idx_submissions_user_id_created_at ON submissions (user_id, created_at DESC);

-- Optimize queue and recovery polling by state
CREATE INDEX IF NOT EXISTS idx_submissions_status ON submissions (status);
