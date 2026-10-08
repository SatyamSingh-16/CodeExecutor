import React from 'react';

export interface MetricsBarProps {
  executionTimeMs?: number | null;
  memoryUsageKb?: number | null;
  exitCode?: number | null;
}

export const MetricsBar: React.FC<MetricsBarProps> = ({
  executionTimeMs,
  memoryUsageKb,
  exitCode,
}) => {
  const formattedTime =
    executionTimeMs !== undefined && executionTimeMs !== null
      ? `${executionTimeMs} ms`
      : '—';

  const formattedMemory =
    memoryUsageKb !== undefined && memoryUsageKb !== null
      ? `${(memoryUsageKb / 1024).toFixed(1)} MB`
      : '—';

  const formattedExitCode =
    exitCode !== undefined && exitCode !== null ? `${exitCode}` : '—';

  const isExitSuccess = exitCode === 0;

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        flexWrap: 'wrap',
        gap: 'var(--space-3)',
        padding: 'var(--space-2) var(--space-3)',
        backgroundColor: 'var(--bg-surface-elevated)',
        borderTop: '1px solid var(--border-default)',
        borderRadius: '0 0 var(--radius-md) var(--radius-md)',
        fontFamily: 'var(--font-mono)',
        fontSize: 'var(--text-xs)',
        color: 'var(--text-secondary)',
      }}
      aria-label="Execution Metrics"
    >
      <div style={{ display: 'flex', gap: 'var(--space-4)', flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', gap: 'var(--space-1-5)', alignItems: 'center' }}>
          <span style={{ color: 'var(--text-muted)' }}>Time:</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 'var(--font-medium)' }}>
            {formattedTime}
          </span>
        </div>

        <div style={{ display: 'flex', gap: 'var(--space-1-5)', alignItems: 'center' }}>
          <span style={{ color: 'var(--text-muted)' }}>Memory:</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 'var(--font-medium)' }}>
            {formattedMemory}
          </span>
        </div>
      </div>

      <div style={{ display: 'flex', gap: 'var(--space-1-5)', alignItems: 'center' }}>
        <span style={{ color: 'var(--text-muted)' }}>Exit Code:</span>
        <span
          style={{
            color:
              exitCode === null || exitCode === undefined
                ? 'var(--text-muted)'
                : isExitSuccess
                ? 'var(--color-success)'
                : 'var(--color-error)',
            fontWeight: 'var(--font-semibold)',
          }}
        >
          {formattedExitCode}
        </span>
      </div>
    </div>
  );
};
