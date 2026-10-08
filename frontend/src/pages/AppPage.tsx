import React, { useCallback, useEffect } from 'react';
import { PageContainer } from '../components/ui/PageContainer';
import { Workspace } from '../workspace/Workspace';
import { useWorkspace } from '../workspace/useWorkspace';
import type { WorkspaceRunRequest } from '../workspace/types';
import { useExecution } from '../execution/useExecution';
import { useSubmissionHistory, HistoryDrawer } from '../history';

export interface AppPageProps {
  onRun?: (request: WorkspaceRunRequest) => void;
}

export const AppPage: React.FC<AppPageProps> = ({ onRun }) => {
  const workspace = useWorkspace();
  const execution = useExecution();
  const history = useSubmissionHistory();

  const handleRun = useCallback(
    async (request: WorkspaceRunRequest) => {
      onRun?.(request);
      await execution.submit(request);
    },
    [execution, onRun]
  );

  // Auto-refresh submission history when execution reaches a terminal status
  useEffect(() => {
    if (execution.status === 'terminal') {
      history.refresh();
    }
  }, [execution.status, history.refresh]);

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
        onOpenHistory={history.openDrawer}
        workspaceState={workspace}
        workspaceActions={workspace}
      />

      <HistoryDrawer
        isOpen={history.isOpen}
        onClose={history.closeDrawer}
        submissions={history.submissions}
        isLoading={history.isLoading}
        error={history.error}
        hasMore={history.hasMore}
        onLoadMore={history.loadMore}
        onRefresh={history.refresh}
        selectedId={history.selectedId}
        selectedSubmission={history.selectedSubmission}
        isLoadingDetail={history.isLoadingDetail}
        detailError={history.detailError}
        onSelectSubmission={history.selectSubmission}
        onClearSelection={history.clearSelection}
        onRestore={workspace.loadSubmission}
        isWorkspaceModified={workspace.isModified}
      />
    </PageContainer>
  );
};
