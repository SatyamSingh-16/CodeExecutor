import { api } from '../api/client';
import type {
  CreateSubmissionRequest,
  CreateSubmissionResponse,
  SubmissionDetail,
} from '../types/submission';

/**
 * Submits code to the distributed code execution backend.
 * POST /api/submissions
 * Response: 202 Accepted with { id, status, created_at }
 */
export async function submitCode(
  request: CreateSubmissionRequest
): Promise<CreateSubmissionResponse> {
  return api.post<CreateSubmissionResponse>('/api/submissions', {
    language: request.language,
    source_code: request.source_code || request.code,
    stdin: request.stdin,
  });
}

/**
 * Fetches the authoritative submission record from the database.
 * GET /api/submissions/:id
 */
export async function getSubmission(id: string): Promise<SubmissionDetail> {
  return api.get<SubmissionDetail>(`/api/submissions/${encodeURIComponent(id)}`);
}
