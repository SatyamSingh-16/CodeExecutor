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

  const sourceCode = codePerLanguage[language] || '';

  return {
    language,
    sourceCode,
    stdin,
    codePerLanguage,
    setLanguage,
    setSourceCode,
    setStdin,
    resetToTemplate,
  };
}
