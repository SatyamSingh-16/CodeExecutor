import { api } from '../api/client';
import type { SubmissionDetail } from '../types/submission';
export { getSubmission } from '../execution/api';

/**
 * Fetches paginated submission history for the authenticated user.
 * GET /api/submissions?page=:page&limit=:limit
 */
export async function listSubmissions(
  page = 1,
  limit = 20
): Promise<SubmissionDetail[]> {
  const query = new URLSearchParams({
    page: String(page),
    limit: String(limit),
  });
  return api.get<SubmissionDetail[]>(`/api/submissions?${query.toString()}`);
}
