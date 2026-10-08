import type { SupportedLanguage } from '../types/submission';

export interface WorkspaceRunRequest {
  language: SupportedLanguage;
  sourceCode: string;
  stdin: string;
}

export interface WorkspaceState {
  language: SupportedLanguage;
  sourceCode: string;
  stdin: string;
  codePerLanguage: Record<SupportedLanguage, string>;
  isModified: boolean;
}

export interface WorkspaceActionHandlers {
  setLanguage: (language: SupportedLanguage) => void;
  setSourceCode: (code: string) => void;
  setStdin: (stdin: string) => void;
  resetToTemplate: () => void;
  loadSubmission: (submission: {
    language: SupportedLanguage;
    sourceCode: string;
    stdin?: string;
  }) => void;
}
