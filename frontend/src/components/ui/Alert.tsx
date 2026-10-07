import React from 'react';

export interface AlertProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: 'info' | 'success' | 'warning' | 'error';
  title?: string;
}

export const Alert: React.FC<AlertProps> = ({
  children,
  variant = 'info',
  title,
  style,
  ...props
}) => {
  const getVariantStyles = (): React.CSSProperties => {
    switch (variant) {
      case 'success':
        return {
          backgroundColor: 'var(--color-success-bg)',
          borderColor: 'var(--color-success-border)',
          color: 'var(--color-success)',
        };
      case 'warning':
        return {
          backgroundColor: 'var(--color-warning-bg)',
          borderColor: 'var(--color-warning-border)',
          color: 'var(--color-warning)',
        };
      case 'error':
        return {
          backgroundColor: 'var(--color-error-bg)',
          borderColor: 'var(--color-error-border)',
          color: 'var(--color-error)',
        };
      case 'info':
      default:
        return {
          backgroundColor: 'var(--color-info-bg)',
          borderColor: 'var(--color-info-border)',
          color: 'var(--color-info)',
        };
    }
  };

  return (
    <div
      role={variant === 'error' ? 'alert' : 'status'}
      style={{
        borderRadius: 'var(--radius-md)',
        borderWidth: '1px',
        borderStyle: 'solid',
        padding: 'var(--space-3) var(--space-4)',
        display: 'flex',
        flexDirection: 'column',
        gap: 'var(--space-1)',
        fontSize: 'var(--text-sm)',
        ...getVariantStyles(),
        ...style,
      }}
      {...props}
    >
      {title && (
        <strong style={{ fontWeight: 'var(--font-semibold)', color: 'inherit' }}>
          {title}
        </strong>
      )}
      <div style={{ color: 'var(--text-secondary)' }}>{children}</div>
    </div>
  );
};
