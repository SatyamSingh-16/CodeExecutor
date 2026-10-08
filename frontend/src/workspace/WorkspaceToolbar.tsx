import React from 'react';
import type { SupportedLanguage } from '../types/submission';
import { LanguageSelector } from './LanguageSelector';
import { Button } from '../components/ui/Button';

export interface WorkspaceToolbarProps {
  language: SupportedLanguage;
  onLanguageChange: (lang: SupportedLanguage) => void;
  onResetTemplate: () => void;
  onRun: () => void;
  isRunning?: boolean;
  canRun?: boolean;
  onOpenHistory?: () => void;
}

export const WorkspaceToolbar: React.FC<WorkspaceToolbarProps> = ({
  language,
  onLanguageChange,
  onResetTemplate,
  onRun,
  isRunning = false,
  canRun = true,
  onOpenHistory,
}) => {
  const isMac = typeof navigator !== 'undefined' && /Mac|iPod|iPhone|iPad/.test(navigator.platform);
  const shortcutHint = isMac ? '⌘+Enter' : 'Ctrl+Enter';

  return (
    <div
      style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        flexWrap: 'wrap',
        gap: 'var(--space-3)',
        padding: 'var(--space-3) var(--space-4)',
        backgroundColor: 'var(--bg-surface)',
        borderRadius: 'var(--radius-md)',
        border: '1px solid var(--border-default)',
        marginBottom: 'var(--space-4)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', flexWrap: 'wrap' }}>
        <LanguageSelector value={language} onChange={onLanguageChange} disabled={isRunning} />

        <Button
          variant="ghost"
          size="sm"
          onClick={onResetTemplate}
          disabled={isRunning}
          title="Reset editor content to language starter boilerplate"
        >
          Reset to Template
        </Button>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
        <span
          style={{
            fontSize: 'var(--text-xs)',
            color: 'var(--text-muted)',
            fontFamily: 'var(--font-mono)',
            display: 'none',
          }}
          className="shortcut-hint"
        >
          {shortcutHint}
        </span>

        {onOpenHistory && (
          <Button
            variant="secondary"
            size="md"
            onClick={onOpenHistory}
            aria-label="Submission History"
          >
            History
          </Button>
        )}

        <Button
          variant="primary"
          size="md"
          onClick={onRun}
          disabled={!canRun || isRunning}
          isLoading={isRunning}
          aria-label="Run Code"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 'var(--space-2)',
            fontWeight: 'var(--font-semibold)',
          }}
        >
          <span>Run Code</span>
          <kbd
            style={{
              fontSize: '10px',
              fontFamily: 'var(--font-mono)',
              padding: '2px 4px',
              borderRadius: 'var(--radius-xs)',
              backgroundColor: 'rgba(255, 255, 255, 0.2)',
              marginLeft: 'var(--space-1)',
            }}
          >
            {shortcutHint}
          </kbd>
        </Button>
      </div>
    </div>
  );
};
