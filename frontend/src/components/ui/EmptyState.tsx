import React from 'react';

export interface EmptyStateProps {
  title: string;
  description?: string;
  icon?: React.ReactNode;
  action?: React.ReactNode;
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  title,
  description,
  icon,
  action,
}) => {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 'var(--space-12) var(--space-6)',
        textAlign: 'center',
        backgroundColor: 'var(--bg-surface)',
        borderRadius: 'var(--radius-lg)',
        border: '1px dashed var(--border-default)',
      }}
    >
      {icon && (
        <div
          style={{
            marginBottom: 'var(--space-4)',
            color: 'var(--text-muted)',
            fontSize: 'var(--text-3xl)',
          }}
        >
          {icon}
        </div>
      )}
      <h3
        style={{
          fontSize: 'var(--text-base)',
          fontWeight: 'var(--font-semibold)',
          color: 'var(--text-primary)',
          marginBottom: description ? 'var(--space-1-5)' : 0,
        }}
      >
        {title}
      </h3>
      {description && (
        <p
          style={{
            fontSize: 'var(--text-sm)',
            color: 'var(--text-secondary)',
            maxWidth: '400px',
            marginBottom: action ? 'var(--space-6)' : 0,
          }}
        >
          {description}
        </p>
      )}
      {action && <div>{action}</div>}
    </div>
  );
};
