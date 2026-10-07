-- 000003_add_submissions_status_created_at_idx.up.sql
-- Targeted composite index to optimize recovery sweeper queries filtering on
-- status = 'QUEUED' ordered by created_at ASC.

CREATE INDEX IF NOT EXISTS idx_submissions_status_created_at ON submissions (status, created_at ASC);
