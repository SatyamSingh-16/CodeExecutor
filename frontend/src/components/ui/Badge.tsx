import React from 'react';
import type { SubmissionStatus } from '../../types/submission';

export type BadgeVariant =
  | 'queued'
  | 'processing'
  | 'success'
  | 'error'
  | 'warning'
  | 'neutral';

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant;
  status?: SubmissionStatus;
}

export const Badge: React.FC<BadgeProps> = ({
  children,
  variant,
  status,
  style,
  ...props
}) => {
  // Determine variant from submission status if provided
  const resolvedVariant: BadgeVariant = React.useMemo(() => {
    if (variant) return variant;
    if (!status) return 'neutral';

    switch (status) {
      case 'QUEUED':
        return 'queued';
      case 'PROCESSING':
        return 'processing';
      case 'SUCCESS':
        return 'success';
      case 'TIME_LIMIT_EXCEEDED':
      case 'MEMORY_LIMIT_EXCEEDED':
        return 'warning';
      case 'COMPILATION_ERROR':
      case 'RUNTIME_ERROR':
      case 'SYSTEM_ERROR':
      default:
        return 'error';
    }
  }, [variant, status]);

  const getVariantStyles = (): React.CSSProperties => {
    switch (resolvedVariant) {
      case 'queued':
        return {
          backgroundColor: 'var(--color-neutral-bg)',
          color: 'var(--color-neutral)',
          borderColor: 'var(--color-neutral-border)',
        };
      case 'processing':
        return {
          backgroundColor: 'var(--color-info-bg)',
          color: 'var(--color-info)',
          borderColor: 'var(--color-info-border)',
        };
      case 'success':
        return {
          backgroundColor: 'var(--color-success-bg)',
          color: 'var(--color-success)',
          borderColor: 'var(--color-success-border)',
        };
      case 'warning':
        return {
          backgroundColor: 'var(--color-warning-bg)',
          color: 'var(--color-warning)',
          borderColor: 'var(--color-warning-border)',
        };
      case 'error':
        return {
          backgroundColor: 'var(--color-error-bg)',
          color: 'var(--color-error)',
          borderColor: 'var(--color-error-border)',
        };
      case 'neutral':
      default:
        return {
          backgroundColor: 'var(--bg-surface-elevated)',
          color: 'var(--text-secondary)',
          borderColor: 'var(--border-default)',
        };
    }
  };

  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 'var(--space-1-5)',
        fontSize: 'var(--text-xs)',
        fontWeight: 'var(--font-medium)',
        fontFamily: 'var(--font-mono)',
        padding: 'var(--space-0-5) var(--space-2)',
        borderRadius: 'var(--radius-sm)',
        borderWidth: '1px',
        borderStyle: 'solid',
        lineHeight: 'var(--leading-none)',
        ...getVariantStyles(),
        ...style,
      }}
      {...props}
    >
      {resolvedVariant === 'processing' && (
        <span
          style={{
            width: '6px',
            height: '6px',
            borderRadius: '50%',
            backgroundColor: 'currentColor',
            display: 'inline-block',
          }}
        />
      )}
      {children || status}
    </span>
  );
};
