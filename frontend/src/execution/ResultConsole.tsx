import React, { useState, useEffect } from 'react';
import { Card } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { Alert } from '../components/ui/Alert';
import { LoadingSpinner } from '../components/ui/LoadingSpinner';
import { MetricsBar } from './MetricsBar';
import type { ExecutionState } from './types';

export interface ResultConsoleProps {
  state: ExecutionState;
}

type TabType = 'stdout' | 'stderr' | 'compilation';

export const ResultConsole: React.FC<ResultConsoleProps> = ({ state }) => {
  const { status, submission, error } = state;
  const [activeTab, setActiveTab] = useState<TabType>('stdout');

  // Automatic tab switching based on submission status
  useEffect(() => {
    if (!submission) return;

    if (submission.status === 'COMPILATION_ERROR' && submission.compilation_output) {
      setActiveTab('compilation');
    } else if (submission.status === 'RUNTIME_ERROR' && submission.stderr) {
      setActiveTab('stderr');
    } else if (submission.status === 'SUCCESS') {
      setActiveTab('stdout');
    }
  }, [submission?.status, submission?.compilation_output, submission?.stderr]);

  // Determine current active content
  const activeContent = React.useMemo(() => {
    if (!submission) return '';
    switch (activeTab) {
      case 'stdout':
        return submission.stdout || '';
      case 'stderr':
        return submission.stderr || '';
      case 'compilation':
        return submission.compilation_output || '';
      default:
        return '';
    }
  }, [submission, activeTab]);

  const isTruncated = React.useMemo(() => {
    if (!submission) return false;
    if (activeTab === 'stdout') return submission.stdout_truncated;
    if (activeTab === 'stderr') return submission.stderr_truncated;
    return false;
  }, [submission, activeTab]);

  const renderStatusBadge = () => {
    if (status === 'submitting') {
      return (
        <Badge variant="processing">
          <LoadingSpinner size="sm" />
          <span>Submitting...</span>
        </Badge>
      );
    }
    if (status === 'queued') {
      return (
        <Badge status="QUEUED">
          <span>QUEUED</span>
        </Badge>
      );
    }
    if (status === 'processing') {
      return (
        <Badge status="PROCESSING">
          <span>PROCESSING</span>
        </Badge>
      );
    }
    if (submission) {
      return <Badge status={submission.status} />;
    }
    if (status === 'error') {
      return <Badge variant="error">ERROR</Badge>;
    }
    return <Badge variant="neutral">READY</Badge>;
  };

  const hasCompilationOutput = Boolean(submission?.compilation_output);
  const hasStderr = Boolean(submission?.stderr);

  return (
    <Card
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        minHeight: '280px',
        padding: 0,
        overflow: 'hidden',
      }}
      role="region"
      aria-label="Execution Output Console"
    >
      {/* Header bar */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: 'var(--space-3) var(--space-4)',
          borderBottom: '1px solid var(--border-default)',
          backgroundColor: 'var(--bg-surface)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <h3
            style={{
              fontSize: 'var(--text-sm)',
              fontWeight: 'var(--font-semibold)',
              margin: 0,
              color: 'var(--text-primary)',
            }}
          >
            Execution Output
          </h3>
        </div>
        <div aria-live="polite" style={{ display: 'flex', alignItems: 'center' }}>
          {renderStatusBadge()}
        </div>
      </div>

      {/* Tabs navigation */}
      <div
        style={{
          display: 'flex',
          borderBottom: '1px solid var(--border-default)',
          backgroundColor: 'var(--bg-surface-elevated)',
          padding: '0 var(--space-2)',
          gap: 'var(--space-1)',
        }}
        role="tablist"
        aria-label="Output Streams"
      >
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'stdout'}
          onClick={() => setActiveTab('stdout')}
          style={{
            padding: 'var(--space-2) var(--space-3)',
            fontSize: 'var(--text-xs)',
            fontFamily: 'var(--font-mono)',
            fontWeight: activeTab === 'stdout' ? 'var(--font-semibold)' : 'var(--font-normal)',
            color: activeTab === 'stdout' ? 'var(--color-primary)' : 'var(--text-secondary)',
            backgroundColor: 'transparent',
            border: 'none',
            borderBottom: activeTab === 'stdout' ? '2px solid var(--color-primary)' : '2px solid transparent',
            cursor: 'pointer',
          }}
        >
          Standard Output
        </button>

        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'stderr'}
          onClick={() => setActiveTab('stderr')}
          style={{
            padding: 'var(--space-2) var(--space-3)',
            fontSize: 'var(--text-xs)',
            fontFamily: 'var(--font-mono)',
            fontWeight: activeTab === 'stderr' ? 'var(--font-semibold)' : 'var(--font-normal)',
            color:
              activeTab === 'stderr'
                ? 'var(--color-error)'
                : hasStderr
                ? 'var(--color-error)'
                : 'var(--text-secondary)',
            backgroundColor: 'transparent',
            border: 'none',
            borderBottom: activeTab === 'stderr' ? '2px solid var(--color-error)' : '2px solid transparent',
            cursor: 'pointer',
          }}
        >
          Standard Error {hasStderr && '•'}
        </button>

        {(hasCompilationOutput || activeTab === 'compilation') && (
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'compilation'}
            onClick={() => setActiveTab('compilation')}
            style={{
              padding: 'var(--space-2) var(--space-3)',
              fontSize: 'var(--text-xs)',
              fontFamily: 'var(--font-mono)',
              fontWeight: activeTab === 'compilation' ? 'var(--font-semibold)' : 'var(--font-normal)',
              color: activeTab === 'compilation' ? 'var(--color-warning)' : 'var(--text-secondary)',
              backgroundColor: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'compilation' ? '2px solid var(--color-warning)' : '2px solid transparent',
              cursor: 'pointer',
            }}
          >
            Compilation Output {hasCompilationOutput && '•'}
          </button>
        )}
      </div>

      {/* Truncation notification banner */}
      {isTruncated && (
        <div style={{ padding: 'var(--space-2) var(--space-3)', backgroundColor: 'var(--color-warning-bg)' }}>
          <Alert variant="warning">
            Output truncated at 64 KB limit.
          </Alert>
        </div>
      )}

      {/* Main Console Output Viewport */}
      <div
        style={{
          flex: 1,
          padding: 'var(--space-3)',
          backgroundColor: 'var(--bg-canvas)',
          overflowY: 'auto',
          minHeight: '140px',
        }}
      >
        {error && (
          <div style={{ marginBottom: 'var(--space-2)' }}>
            <Alert variant="error" title="Execution Error">
              {error}
            </Alert>
          </div>
        )}

        {status === 'idle' && !submission && !error && (
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              minHeight: '120px',
              color: 'var(--text-muted)',
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--text-xs)',
              textAlign: 'center',
              gap: 'var(--space-1)',
            }}
          >
            <span>Ready to Execute</span>
            <span>Click "Run Code" or press ⌘+Enter to submit your program.</span>
          </div>
        )}

        {status === 'submitting' && (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              minHeight: '120px',
              gap: 'var(--space-2)',
              color: 'var(--text-secondary)',
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--text-xs)',
            }}
          >
            <LoadingSpinner size="md" />
            <span>Submitting job to execution queue...</span>
          </div>
        )}

        {status === 'queued' && (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              minHeight: '120px',
              gap: 'var(--space-2)',
              color: 'var(--text-secondary)',
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--text-xs)',
            }}
          >
            <LoadingSpinner size="md" />
            <span>Job queued. Waiting for an available worker...</span>
          </div>
        )}

        {status === 'processing' && !activeContent && (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              minHeight: '120px',
              gap: 'var(--space-2)',
              color: 'var(--text-secondary)',
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--text-xs)',
            }}
          >
            <LoadingSpinner size="md" />
            <span>Executing inside isolated container sandbox...</span>
          </div>
        )}

        {/* Informative Status Explanations for limit exceedances */}
        {submission?.status === 'TIME_LIMIT_EXCEEDED' && (
          <div style={{ marginBottom: 'var(--space-2)' }}>
            <Alert variant="warning" title="Time Limit Exceeded">
              The program exceeded the execution time limit (SIGKILL enforced).
            </Alert>
          </div>
        )}

        {submission?.status === 'MEMORY_LIMIT_EXCEEDED' && (
          <div style={{ marginBottom: 'var(--space-2)' }}>
            <Alert variant="warning" title="Memory Limit Exceeded">
              The program exceeded the memory allocation limit and was terminated by the cgroup OOM killer.
            </Alert>
          </div>
        )}

        {submission?.status === 'SYSTEM_ERROR' && (
          <div style={{ marginBottom: 'var(--space-2)' }}>
            <Alert variant="error" title="System Error">
              An infrastructure error occurred during container execution.
            </Alert>
          </div>
        )}

        {/* Monospace output text */}
        {(activeContent || (submission && status === 'terminal')) && (
          <pre
            style={{
              margin: 0,
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--text-xs)',
              lineHeight: 'var(--leading-relaxed)',
              color:
                activeTab === 'stderr'
                  ? 'var(--color-error)'
                  : activeTab === 'compilation'
                  ? 'var(--color-warning)'
                  : 'var(--text-primary)',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-all',
            }}
            tabIndex={0}
            aria-label={`${activeTab} console output`}
          >
            {activeContent || (status === 'terminal' ? '(No output generated)' : '')}
          </pre>
        )}
      </div>

      {/* Metrics footer */}
      <MetricsBar
        executionTimeMs={submission?.execution_time_ms}
        memoryUsageKb={submission?.memory_usage_kb}
        exitCode={submission?.exit_code}
      />
    </Card>
  );
};
