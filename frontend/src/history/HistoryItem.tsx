import React from 'react';
import type { SubmissionDetail } from '../types/submission';
import { Badge } from '../components/ui/Badge';
import { formatRelativeTime } from './time';

export interface HistoryItemProps {
  submission: SubmissionDetail;
  isSelected?: boolean;
  onSelect: (submission: SubmissionDetail) => void;
}

export const HistoryItem: React.FC<HistoryItemProps> = ({
  submission,
  isSelected = false,
  onSelect,
}) => {
  const languageLabel = submission.language === 'python' ? 'Python 3' : 'Go';
  const relativeTime = formatRelativeTime(submission.created_at);

  const formattedTime =
    submission.execution_time_ms !== null && submission.execution_time_ms !== undefined
      ? `${submission.execution_time_ms} ms`
      : null;

  const formattedMemory =
    submission.memory_usage_kb !== null && submission.memory_usage_kb !== undefined
      ? `${(submission.memory_usage_kb / 1024).toFixed(1)} MB`
      : null;

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onSelect(submission)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSelect(submission);
        }
      }}
      aria-label={`Submission ${submission.id.slice(0, 8)}, ${submission.status}, ${languageLabel}, ${relativeTime}`}
      style={{
        padding: 'var(--space-3) var(--space-4)',
        borderRadius: 'var(--radius-md)',
        backgroundColor: isSelected ? 'var(--bg-surface-elevated)' : 'var(--bg-surface)',
        border: `1px solid ${isSelected ? 'var(--color-primary)' : 'var(--border-default)'}`,
        cursor: 'pointer',
        display: 'flex',
        flexDirection: 'column',
        gap: 'var(--space-2)',
        transition: 'all 0.15s ease-in-out',
        outline: 'none',
      }}
      onMouseEnter={(e) => {
        if (!isSelected) {
          e.currentTarget.style.borderColor = 'var(--border-hover)';
          e.currentTarget.style.backgroundColor = 'var(--bg-surface-elevated)';
        }
      }}
      onMouseLeave={(e) => {
        if (!isSelected) {
          e.currentTarget.style.borderColor = 'var(--border-default)';
          e.currentTarget.style.backgroundColor = 'var(--bg-surface)';
        }
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Badge status={submission.status} />
        <span
          style={{
            fontSize: 'var(--text-xs)',
            color: 'var(--text-muted)',
            fontFamily: 'var(--font-mono)',
          }}
        >
          {relativeTime}
        </span>
      </div>

      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          fontSize: 'var(--text-xs)',
          color: 'var(--text-secondary)',
        }}
      >
        <span style={{ fontWeight: 'var(--font-semibold)', color: 'var(--text-primary)' }}>
          {languageLabel}
        </span>

        {(formattedTime || formattedMemory) && (
          <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
            {[formattedTime, formattedMemory].filter(Boolean).join(' · ')}
          </span>
        )}
      </div>
    </div>
  );
};
