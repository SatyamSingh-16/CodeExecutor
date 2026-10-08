import React, { useRef } from 'react';
import Editor, { OnMount } from '@monaco-editor/react';
import type { SupportedLanguage } from '../types/submission';
import { defineMonacoTheme, MONACO_THEME_NAME } from './theme';
import { LoadingSpinner } from '../components/ui/LoadingSpinner';

export interface CodeEditorProps {
  language: SupportedLanguage;
  value: string;
  onChange: (value: string) => void;
  onRun?: () => void;
  readOnly?: boolean;
  height?: string | number;
}

export const CodeEditor: React.FC<CodeEditorProps> = React.memo(({
  language,
  value,
  onChange,
  onRun,
  readOnly = false,
  height = '100%',
}) => {
  const onRunRef = useRef(onRun);
  onRunRef.current = onRun;

  const handleEditorDidMount: OnMount = (editor, monaco) => {
    defineMonacoTheme(monaco);
    monaco.editor.setTheme(MONACO_THEME_NAME);

    // Register Cmd+Enter / Ctrl+Enter shortcut to trigger execution
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
      onRunRef.current?.();
    });
  };

  return (
    <div
      style={{
        width: '100%',
        height: height,
        borderRadius: 'var(--radius-md)',
        overflow: 'hidden',
        border: '1px solid var(--border-default)',
        backgroundColor: 'var(--bg-surface)',
        display: 'flex',
        flexDirection: 'column',
      }}
      data-testid="monaco-editor-container"
    >
      <Editor
        height="100%"
        language={language}
        value={value}
        theme="vs-dark"
        onChange={(val) => onChange(val || '')}
        onMount={handleEditorDidMount}
        loading={
          <div
            role="status"
            aria-live="polite"
            style={{
              height: '100%',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              backgroundColor: 'var(--bg-surface)',
              gap: 'var(--space-2)',
            }}
          >
            <LoadingSpinner size="md" label="Loading code editor..." />
            <span style={{ color: 'var(--text-muted)', fontSize: 'var(--text-xs)' }}>
              Initializing Monaco Editor...
            </span>
          </div>
        }
        options={{
          readOnly,
          minimap: { enabled: false },
          fontSize: 14,
          fontFamily: "'Fira Code', 'JetBrains Mono', Menlo, Monaco, Consolas, monospace",
          fontLigatures: true,
          tabSize: language === 'go' ? 4 : 4,
          insertSpaces: language !== 'go',
          automaticLayout: true,
          scrollBeyondLastLine: false,
          wordWrap: 'on',
          lineNumbers: 'on',
          lineNumbersMinChars: 3,
          renderLineHighlight: 'all',
          cursorBlinking: 'smooth',
          smoothScrolling: true,
          padding: { top: 12, bottom: 12 },
          accessibilitySupport: 'on',
        }}
      />
    </div>
  );
});
