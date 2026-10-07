export const MONACO_THEME_NAME = 'code-executor-dark';

export function defineMonacoTheme(monaco: any): void {
  if (!monaco || !monaco.editor) return;

  monaco.editor.defineTheme(MONACO_THEME_NAME, {
    base: 'vs-dark',
    inherit: true,
    rules: [
      { token: 'comment', foreground: '64748b', fontStyle: 'italic' },
      { token: 'keyword', foreground: '38bdf8', fontStyle: 'bold' },
      { token: 'string', foreground: '34d399' },
      { token: 'number', foreground: 'f59e0b' },
      { token: 'type', foreground: 'a78bfa' },
      { token: 'function', foreground: '60a5fa' },
      { token: 'delimiter', foreground: '94a3b8' },
    ],
    colors: {
      'editor.background': '#0f172a',
      'editor.foreground': '#f8fafc',
      'editorCursor.foreground': '#38bdf8',
      'editor.lineHighlightBackground': '#1e293b66',
      'editorLineNumber.foreground': '#475569',
      'editorLineNumber.activeForeground': '#94a3b8',
      'editor.selectionBackground': '#3b82f640',
      'editor.inactiveSelectionBackground': '#3b82f620',
      'editorIndentGuide.background': '#1e293b',
      'editorIndentGuide.activeBackground': '#334155',
    },
  });
}
