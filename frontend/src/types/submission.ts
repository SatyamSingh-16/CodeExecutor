export type SupportedLanguage = 'python' | 'go';

export type SubmissionStatus =
  | 'QUEUED'
  | 'PROCESSING'
  | 'SUCCESS'
  | 'COMPILATION_ERROR'
  | 'RUNTIME_ERROR'
  | 'TIME_LIMIT_EXCEEDED'
  | 'MEMORY_LIMIT_EXCEEDED'
  | 'SYSTEM_ERROR';

export interface CreateSubmissionRequest {
  language: SupportedLanguage;
  source_code: string;
  code?: string;
  stdin?: string;
}

export interface CreateSubmissionResponse {
  id: string;
  status: SubmissionStatus;
  created_at?: string;
}

export interface SubmissionDetail {
  id: string;
  language: SupportedLanguage;
  status: SubmissionStatus;
  source_code?: string;
  stdin?: string;
  stdout: string;
  stderr: string;
  compilation_output: string;
  stdout_truncated: boolean;
  stderr_truncated: boolean;
  exit_code: number | null;
  execution_time_ms: number | null;
  memory_usage_kb: number | null;
  created_at: string;
  updated_at: string;
}

export function isTerminalStatus(status: SubmissionStatus): boolean {
  switch (status) {
    case 'SUCCESS':
    case 'COMPILATION_ERROR':
    case 'RUNTIME_ERROR':
    case 'TIME_LIMIT_EXCEEDED':
    case 'MEMORY_LIMIT_EXCEEDED':
    case 'SYSTEM_ERROR':
      return true;
    default:
      return false;
  }
}
