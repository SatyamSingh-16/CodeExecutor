-- 000003_add_submissions_status_created_at_idx.down.sql
-- Reverts the composite index on submissions (status, created_at ASC).

DROP INDEX IF EXISTS idx_submissions_status_created_at;
