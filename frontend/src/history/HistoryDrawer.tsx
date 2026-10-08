import React, { useState, useEffect, useCallback } from 'react';
import type { SubmissionDetail } from '../types/submission';
import type { WorkspaceRestoreData } from './types';
import { HistoryItem } from './HistoryItem';
import { formatRelativeTime } from './time';
import { Badge } from '../components/ui/Badge';
import { Button } from '../components/ui/Button';
import { Alert } from '../components/ui/Alert';
import { LoadingSpinner } from '../components/ui/LoadingSpinner';
import { EmptyState } from '../components/ui/EmptyState';
import { MetricsBar } from '../execution/MetricsBar';

export interface HistoryDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  submissions: SubmissionDetail[];
  isLoading: boolean;
  error: string | null;
  hasMore: boolean;
  onLoadMore: () => void;
  onRefresh: () => void;
  selectedId: string | null;
  selectedSubmission: SubmissionDetail | null;
  isLoadingDetail: boolean;
  detailError: string | null;
  onSelectSubmission: (id: string) => void;
  onClearSelection: () => void;
  onRestore: (data: WorkspaceRestoreData) => void;
  isWorkspaceModified?: boolean;
}

type TabType = 'stdout' | 'stderr' | 'compilation' | 'code';

export const HistoryDrawer: React.FC<HistoryDrawerProps> = ({
  isOpen,
  onClose,
  submissions,
  isLoading,
  error,
  hasMore,
  onLoadMore,
  onRefresh,
  selectedId,
  selectedSubmission,
  isLoadingDetail,
  detailError,
  onSelectSubmission,
  onClearSelection,
  onRestore,
  isWorkspaceModified = false,
}) => {
  const [showConfirm, setShowConfirm] = useState(false);
  const [activeTab, setActiveTab] = useState<TabType>('stdout');

  // Handle Escape key to close drawer
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  // Reset confirmation and tab selection when selected submission changes
  useEffect(() => {
    setShowConfirm(false);
    if (selectedSubmission) {
      if (selectedSubmission.status === 'COMPILATION_ERROR' && selectedSubmission.compilation_output) {
        setActiveTab('compilation');
      } else if (selectedSubmission.status === 'RUNTIME_ERROR' && selectedSubmission.stderr) {
        setActiveTab('stderr');
      } else {
        setActiveTab('stdout');
      }
    }
  }, [selectedSubmission]);

  const handleRestoreClick = () => {
    if (isWorkspaceModified) {
      setShowConfirm(true);
    } else {
      executeRestore();
    }
  };

  const executeRestore = useCallback(() => {
    if (!selectedSubmission) return;
    onRestore({
      language: selectedSubmission.language,
      sourceCode: selectedSubmission.source_code || '',
      stdin: selectedSubmission.stdin || '',
    });
    setShowConfirm(false);
    onClose();
  }, [selectedSubmission, onRestore, onClose]);

  if (!isOpen) return null;

  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: 100,
        display: 'flex',
        justifyContent: 'flex-end',
      }}
      role="dialog"
      aria-modal="true"
      aria-label="Submission History Drawer"
    >
      {/* Backdrop */}
      <div
        style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: 'rgba(0, 0, 0, 0.6)',
          backdropFilter: 'blur(2px)',
        }}
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Drawer Panel */}
      <div
        style={{
          position: 'relative',
          width: 'min(460px, 100vw)',
          height: '100%',
          backgroundColor: 'var(--bg-surface)',
          borderLeft: '1px solid var(--border-default)',
          boxShadow: 'var(--shadow-xl)',
          display: 'flex',
          flexDirection: 'column',
          zIndex: 101,
          animation: 'slideInRight 0.2s ease-out',
        }}
      >
        {/* Header */}
        <div
          style={{
            padding: 'var(--space-4)',
            borderBottom: '1px solid var(--border-default)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 'var(--space-3)',
            backgroundColor: 'var(--bg-surface-elevated)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
            {selectedId && (
              <Button
                variant="ghost"
                size="sm"
                onClick={onClearSelection}
                aria-label="Back to History List"
                style={{ padding: 'var(--space-1) var(--space-2)' }}
              >
                ← Back
              </Button>
            )}
            <h2 style={{ fontSize: 'var(--text-base)', margin: 0, color: 'var(--text-primary)' }}>
              {selectedId ? 'Submission Detail' : 'Submission History'}
            </h2>
          </div>

          <Button
            variant="ghost"
            size="sm"
            onClick={onClose}
            aria-label="Close History"
            style={{ fontSize: 'var(--text-sm)' }}
          >
            ✕
          </Button>
        </div>

        {/* Drawer Content Body */}
        <div
          style={{
            flex: 1,
            overflowY: 'auto',
            padding: 'var(--space-4)',
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-3)',
          }}
        >
          {/* DETAIL INSPECTION VIEW */}
          {selectedId ? (
            <div>
              {isLoadingDetail ? (
                <div
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    padding: 'var(--space-12) 0',
                    gap: 'var(--space-2)',
                  }}
                >
                  <LoadingSpinner size="md" />
                  <span style={{ fontSize: 'var(--text-xs)', color: 'var(--text-muted)' }}>
                    Loading submission details...
                  </span>
                </div>
              ) : detailError ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
                  <Alert variant="error" title="Error Loading Detail">
                    {detailError}
                  </Alert>
                  <Button variant="secondary" size="sm" onClick={() => onSelectSubmission(selectedId)}>
                    Retry
                  </Button>
                </div>
              ) : selectedSubmission ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
                  {/* Top metadata summary */}
                  <div
                    style={{
                      padding: 'var(--space-3)',
                      backgroundColor: 'var(--bg-surface-elevated)',
                      borderRadius: 'var(--radius-md)',
                      border: '1px solid var(--border-default)',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 'var(--space-2)',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <Badge status={selectedSubmission.status} />
                      <span
                        style={{
                          fontSize: 'var(--text-xs)',
                          fontFamily: 'var(--font-mono)',
                          color: 'var(--text-muted)',
                        }}
                      >
                        {formatRelativeTime(selectedSubmission.created_at)}
                      </span>
                    </div>

                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 'var(--text-xs)' }}>
                      <span style={{ color: 'var(--text-secondary)' }}>
                        Language: <strong>{selectedSubmission.language === 'python' ? 'Python 3' : 'Go'}</strong>
                      </span>
                      <span
                        style={{
                          fontFamily: 'var(--font-mono)',
                          color: 'var(--text-muted)',
                        }}
                      >
                        ID: {selectedSubmission.id.slice(0, 8)}
                      </span>
                    </div>

                    {/* Restore Action */}
                    <div style={{ marginTop: 'var(--space-2)' }}>
                      {showConfirm ? (
                        <div
                          style={{
                            padding: 'var(--space-3)',
                            backgroundColor: 'var(--color-warning-bg)',
                            borderRadius: 'var(--radius-sm)',
                            border: '1px solid var(--color-warning-border)',
                            display: 'flex',
                            flexDirection: 'column',
                            gap: 'var(--space-2)',
                          }}
                        >
                          <span style={{ fontSize: 'var(--text-xs)', color: 'var(--text-primary)' }}>
                            Replace current editor contents with this submission?
                          </span>
                          <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                            <Button variant="primary" size="sm" onClick={executeRestore}>
                              Load into Editor
                            </Button>
                            <Button variant="secondary" size="sm" onClick={() => setShowConfirm(false)}>
                              Cancel
                            </Button>
                          </div>
                        </div>
                      ) : (
                        <Button
                          variant="primary"
                          size="sm"
                          onClick={handleRestoreClick}
                          style={{ width: '100%' }}
                        >
                          Load into Editor
                        </Button>
                      )}
                    </div>
                  </div>

                  {/* Inspection Tabs */}
                  <div
                    style={{
                      display: 'flex',
                      borderBottom: '1px solid var(--border-default)',
                      backgroundColor: 'var(--bg-surface-elevated)',
                      borderRadius: 'var(--radius-sm) var(--radius-sm) 0 0',
                      gap: 'var(--space-1)',
                      padding: '0 var(--space-2)',
                    }}
                    role="tablist"
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
                        borderBottom: activeTab === 'stdout' ? '2px solid var(--color-primary)' : 'none',
                        cursor: 'pointer',
                      }}
                    >
                      stdout
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
                        color: activeTab === 'stderr' ? 'var(--color-error)' : 'var(--text-secondary)',
                        backgroundColor: 'transparent',
                        border: 'none',
                        borderBottom: activeTab === 'stderr' ? '2px solid var(--color-error)' : 'none',
                        cursor: 'pointer',
                      }}
                    >
                      stderr {selectedSubmission.stderr && '•'}
                    </button>

                    {selectedSubmission.compilation_output && (
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
                          borderBottom: activeTab === 'compilation' ? '2px solid var(--color-warning)' : 'none',
                          cursor: 'pointer',
                        }}
                      >
                        compilation •
                      </button>
                    )}

                    <button
                      type="button"
                      role="tab"
                      aria-selected={activeTab === 'code'}
                      onClick={() => setActiveTab('code')}
                      style={{
                        padding: 'var(--space-2) var(--space-3)',
                        fontSize: 'var(--text-xs)',
                        fontFamily: 'var(--font-mono)',
                        fontWeight: activeTab === 'code' ? 'var(--font-semibold)' : 'var(--font-normal)',
                        color: activeTab === 'code' ? 'var(--color-primary)' : 'var(--text-secondary)',
                        backgroundColor: 'transparent',
                        border: 'none',
                        borderBottom: activeTab === 'code' ? '2px solid var(--color-primary)' : 'none',
                        cursor: 'pointer',
                      }}
                    >
                      Source Code
                    </button>
                  </div>

                  {/* Output content area */}
                  <div
                    style={{
                      padding: 'var(--space-3)',
                      backgroundColor: 'var(--bg-canvas)',
                      borderRadius: '0 0 var(--radius-sm) var(--radius-sm)',
                      border: '1px solid var(--border-default)',
                      borderTop: 'none',
                      minHeight: '120px',
                      maxHeight: '220px',
                      overflowY: 'auto',
                    }}
                  >
                    {activeTab === 'stdout' && (
                      <>
                        {selectedSubmission.stdout_truncated && (
                          <div style={{ marginBottom: 'var(--space-2)' }}>
                            <Alert variant="warning">Output truncated at 64 KB limit.</Alert>
                          </div>
                        )}
                        <pre
                          style={{
                            margin: 0,
                            fontFamily: 'var(--font-mono)',
                            fontSize: 'var(--text-xs)',
                            color: 'var(--text-primary)',
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-all',
                          }}
                        >
                          {selectedSubmission.stdout || '(No standard output)'}
                        </pre>
                      </>
                    )}

                    {activeTab === 'stderr' && (
                      <>
                        {selectedSubmission.stderr_truncated && (
                          <div style={{ marginBottom: 'var(--space-2)' }}>
                            <Alert variant="warning">Error output truncated at 64 KB limit.</Alert>
                          </div>
                        )}
                        <pre
                          style={{
                            margin: 0,
                            fontFamily: 'var(--font-mono)',
                            fontSize: 'var(--text-xs)',
                            color: 'var(--color-error)',
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-all',
                          }}
                        >
                          {selectedSubmission.stderr || '(No standard error)'}
                        </pre>
                      </>
                    )}

                    {activeTab === 'compilation' && (
                      <pre
                        style={{
                          margin: 0,
                          fontFamily: 'var(--font-mono)',
                          fontSize: 'var(--text-xs)',
                          color: 'var(--color-warning)',
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-all',
                        }}
                      >
                        {selectedSubmission.compilation_output || '(No compilation output)'}
                      </pre>
                    )}

                    {activeTab === 'code' && (
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                        <pre
                          style={{
                            margin: 0,
                            fontFamily: 'var(--font-mono)',
                            fontSize: 'var(--text-xs)',
                            color: 'var(--text-primary)',
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-all',
                          }}
                        >
                          {selectedSubmission.source_code || '(No code recorded)'}
                        </pre>
                        {selectedSubmission.stdin && (
                          <div style={{ marginTop: 'var(--space-2)', borderTop: '1px dashed var(--border-default)', paddingTop: 'var(--space-2)' }}>
                            <span style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                              stdin:
                            </span>
                            <pre
                              style={{
                                margin: 0,
                                fontFamily: 'var(--font-mono)',
                                fontSize: 'var(--text-xs)',
                                color: 'var(--text-secondary)',
                                whiteSpace: 'pre-wrap',
                              }}
                            >
                              {selectedSubmission.stdin}
                            </pre>
                          </div>
                        )}
                      </div>
                    )}
                  </div>

                  {/* Metrics bar */}
                  <MetricsBar
                    executionTimeMs={selectedSubmission.execution_time_ms}
                    memoryUsageKb={selectedSubmission.memory_usage_kb}
                    exitCode={selectedSubmission.exit_code}
                  />
                </div>
              ) : null}
            </div>
          ) : (
            /* SUBMISSIONS LIST VIEW */
            <div>
              {isLoading && submissions.length === 0 ? (
                <div
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    padding: 'var(--space-12) 0',
                    gap: 'var(--space-2)',
                  }}
                >
                  <LoadingSpinner size="md" />
                  <span style={{ fontSize: 'var(--text-xs)', color: 'var(--text-muted)' }}>
                    Loading past submissions...
                  </span>
                </div>
              ) : error ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
                  <Alert variant="error" title="Failed to Load History">
                    {error}
                  </Alert>
                  <Button variant="secondary" size="sm" onClick={onRefresh}>
                    Retry
                  </Button>
                </div>
              ) : submissions.length === 0 ? (
                <EmptyState
                  title="No submissions yet"
                  description="Submitted programs and their execution metrics will appear here."
                />
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                  {submissions.map((sub) => (
                    <HistoryItem
                      key={sub.id}
                      submission={sub}
                      isSelected={sub.id === selectedId}
                      onSelect={() => onSelectSubmission(sub.id)}
                    />
                  ))}

                  {hasMore && (
                    <div style={{ marginTop: 'var(--space-3)', textAlign: 'center' }}>
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={onLoadMore}
                        disabled={isLoading}
                        isLoading={isLoading}
                        style={{ width: '100%' }}
                      >
                        Load More
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
