import React, { useState } from 'react';
import type { WorkspaceRunRequest } from './types';
import { useWorkspace, UseWorkspaceOptions } from './useWorkspace';
import { WorkspaceToolbar } from './WorkspaceToolbar';
import { CodeEditor } from './CodeEditor';
import { StdinPanel } from './StdinPanel';
import { ResultConsole } from '../execution/ResultConsole';
import type { ExecutionState } from '../execution/types';
import { Alert } from '../components/ui/Alert';

export interface WorkspaceProps extends UseWorkspaceOptions {
  onRun?: (request: WorkspaceRunRequest) => void;
  isRunning?: boolean;
  executionState?: ExecutionState;
}

export const Workspace: React.FC<WorkspaceProps> = ({
  initialLanguage,
  initialStdin,
  onRun,
  isRunning = false,
  executionState,
}) => {
  const {
    language,
    sourceCode,
    stdin,
    setLanguage,
    setSourceCode,
    setStdin,
    resetToTemplate,
  } = useWorkspace({ initialLanguage, initialStdin });

  const [validationError, setValidationError] = useState<string | null>(null);

  const handleRun = () => {
    setValidationError(null);

    if (!sourceCode.trim()) {
      setValidationError('Cannot execute empty source code. Please write code or reset to starter boilerplate.');
      return;
    }

    if (onRun) {
      onRun({
        language,
        sourceCode,
        stdin,
      });
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
      <WorkspaceToolbar
        language={language}
        onLanguageChange={(newLang) => {
          setLanguage(newLang);
          setValidationError(null);
        }}
        onResetTemplate={resetToTemplate}
        onRun={handleRun}
        isRunning={isRunning}
        canRun={true}
      />

      {validationError && (
        <div style={{ marginBottom: 'var(--space-3)' }}>
          <Alert variant="warning" title="Validation Warning">
            {validationError}
          </Alert>
        </div>
      )}

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'minmax(300px, 1.4fr) minmax(280px, 1fr)',
          gap: 'var(--space-4)',
          minHeight: '560px',
        }}
        className="workspace-grid"
      >
        {/* Primary Editor Pane */}
        <div style={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: '520px' }}>
          <CodeEditor
            language={language}
            value={sourceCode}
            onChange={(val) => {
              setSourceCode(val);
              if (validationError && val.trim().length > 0) {
                setValidationError(null);
              }
            }}
            onRun={handleRun}
          />
        </div>

        {/* Secondary Inputs & Results Column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          {/* Standard Input Panel */}
          <div style={{ flex: '1 1 200px', minHeight: '180px' }}>
            <StdinPanel value={stdin} onChange={setStdin} disabled={isRunning} />
          </div>

          {/* Execution Results Console */}
          <div style={{ flex: '1 1 280px' }}>
            <ResultConsole
              state={
                executionState || {
                  status: isRunning ? 'processing' : 'idle',
                  submissionId: null,
                  submission: null,
                  error: null,
                  isRunning,
                }
              }
            />
          </div>
        </div>
      </div>
    </div>
  );
};
