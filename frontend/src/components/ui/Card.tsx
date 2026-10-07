import React from 'react';

export interface CardProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: 'default' | 'elevated' | 'bordered';
}

export const Card: React.FC<CardProps> = ({
  children,
  variant = 'default',
  style,
  className = '',
  ...props
}) => {
  const getVariantStyles = (): React.CSSProperties => {
    switch (variant) {
      case 'elevated':
        return {
          backgroundColor: 'var(--bg-surface-elevated)',
          border: '1px solid var(--border-subtle)',
          boxShadow: 'var(--shadow-md)',
        };
      case 'bordered':
        return {
          backgroundColor: 'transparent',
          border: '1px solid var(--border-default)',
        };
      case 'default':
      default:
        return {
          backgroundColor: 'var(--bg-surface)',
          border: '1px solid var(--border-default)',
          boxShadow: 'var(--shadow-sm)',
        };
    }
  };

  return (
    <div
      style={{
        borderRadius: 'var(--radius-lg)',
        padding: 'var(--space-6)',
        ...getVariantStyles(),
        ...style,
      }}
      className={className}
      {...props}
    >
      {children}
    </div>
  );
};
