import type { SupportedLanguage, SubmissionStatus } from '../types/submission';

export interface SubmissionEventPayload {
  id: string;
  status: SubmissionStatus;
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

export type ExecutionStatus =
  | 'idle'
  | 'submitting'
  | 'queued'
  | 'processing'
  | 'terminal'
  | 'error';

export interface ExecutionState {
  status: ExecutionStatus;
  submissionId: string | null;
  submission: SubmissionEventPayload | null;
  error: string | null;
  isRunning: boolean;
}

export interface RunCodeRequest {
  language: SupportedLanguage;
  sourceCode: string;
  stdin?: string;
}
