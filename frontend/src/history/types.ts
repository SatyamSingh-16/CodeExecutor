import type { SubmissionDetail, SupportedLanguage } from '../types/submission';

export interface HistoryListResponse {
  submissions: SubmissionDetail[];
  total?: number;
}

export interface WorkspaceRestoreData {
  language: SupportedLanguage;
  sourceCode: string;
  stdin: string;
}
