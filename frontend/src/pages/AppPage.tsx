import React, { useCallback } from 'react';
import { PageContainer } from '../components/ui/PageContainer';
import { Workspace } from '../workspace/Workspace';
import type { WorkspaceRunRequest } from '../workspace/types';
import { useExecution } from '../execution/useExecution';

export interface AppPageProps {
  onRun?: (request: WorkspaceRunRequest) => void;
}

export const AppPage: React.FC<AppPageProps> = ({ onRun }) => {
  const execution = useExecution();

  const handleRun = useCallback(
    async (request: WorkspaceRunRequest) => {
      onRun?.(request);
      await execution.submit(request);
    },
    [execution, onRun]
  );

  return (
    <PageContainer
      maxWidth="full"
      style={{
        padding: 'var(--space-4) var(--space-6)',
        display: 'flex',
        flexDirection: 'column',
        flex: 1,
      }}
    >
      <div style={{ marginBottom: 'var(--space-3)' }}>
        <h2>Execution Workspace</h2>
        <p style={{ fontSize: 'var(--text-xs)', color: 'var(--text-muted)' }}>
          Write and run Python 3 and Go programs with custom stdin inputs.
        </p>
      </div>
      <Workspace
        onRun={handleRun}
        isRunning={execution.isRunning}
        executionState={execution.state}
      />
    </PageContainer>
  );
};
