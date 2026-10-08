import { useState, useCallback } from 'react';
import type { SupportedLanguage } from '../types/submission';
import type { WorkspaceState, WorkspaceActionHandlers } from './types';
import { STARTER_TEMPLATES, DEFAULT_LANGUAGE } from './templates';

export interface UseWorkspaceOptions {
  initialLanguage?: SupportedLanguage;
  initialStdin?: string;
}

export function useWorkspace(options: UseWorkspaceOptions = {}): WorkspaceState & WorkspaceActionHandlers {
  const [language, setLanguageState] = useState<SupportedLanguage>(
    options.initialLanguage || DEFAULT_LANGUAGE
  );

  const [codePerLanguage, setCodePerLanguage] = useState<Record<SupportedLanguage, string>>({
    python: STARTER_TEMPLATES.python,
    go: STARTER_TEMPLATES.go,
  });

  const [stdin, setStdin] = useState<string>(options.initialStdin || '');

  const setLanguage = useCallback((newLanguage: SupportedLanguage) => {
    setLanguageState(newLanguage);
  }, []);

  const setSourceCode = useCallback((newCode: string) => {
    setCodePerLanguage((prev) => ({
      ...prev,
      [language]: newCode,
    }));
  }, [language]);

  const resetToTemplate = useCallback(() => {
    setCodePerLanguage((prev) => ({
      ...prev,
      [language]: STARTER_TEMPLATES[language],
    }));
  }, [language]);

  const loadSubmission = useCallback(
    (sub: { language: SupportedLanguage; sourceCode: string; stdin?: string }) => {
      setLanguageState(sub.language);
      setCodePerLanguage((prev) => ({
        ...prev,
        [sub.language]: sub.sourceCode,
      }));
      setStdin(sub.stdin || '');
    },
    []
  );

  const sourceCode = codePerLanguage[language] || '';
  const isModified =
    sourceCode.trim() !== STARTER_TEMPLATES[language].trim() ||
    stdin.trim().length > 0;

  return {
    language,
    sourceCode,
    stdin,
    codePerLanguage,
    isModified,
    setLanguage,
    setSourceCode,
    setStdin,
    resetToTemplate,
    loadSubmission,
  };
}
