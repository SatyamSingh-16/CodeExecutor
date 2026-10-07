import React from 'react';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'outline' | 'danger' | 'ghost';
  size?: 'sm' | 'md' | 'lg';
  isLoading?: boolean;
}

export const Button: React.FC<ButtonProps> = ({
  children,
  variant = 'primary',
  size = 'md',
  isLoading = false,
  disabled,
  className = '',
  style,
  ...props
}) => {
  const getVariantStyles = (): React.CSSProperties => {
    switch (variant) {
      case 'secondary':
        return {
          backgroundColor: 'var(--bg-surface-elevated)',
          color: 'var(--text-primary)',
          borderColor: 'var(--border-default)',
        };
      case 'outline':
        return {
          backgroundColor: 'transparent',
          color: 'var(--text-primary)',
          borderColor: 'var(--border-default)',
        };
      case 'danger':
        return {
          backgroundColor: 'var(--color-error)',
          color: '#ffffff',
          borderColor: 'transparent',
        };
      case 'ghost':
        return {
          backgroundColor: 'transparent',
          color: 'var(--text-secondary)',
          borderColor: 'transparent',
        };
      case 'primary':
      default:
        return {
          backgroundColor: 'var(--accent-primary)',
          color: '#ffffff',
          borderColor: 'transparent',
        };
    }
  };

  const getSizeStyles = (): React.CSSProperties => {
    switch (size) {
      case 'sm':
        return {
          padding: 'var(--space-1-5) var(--space-3)',
          fontSize: 'var(--text-xs)',
          borderRadius: 'var(--radius-sm)',
        };
      case 'lg':
        return {
          padding: 'var(--space-3) var(--space-6)',
          fontSize: 'var(--text-base)',
          borderRadius: 'var(--radius-md)',
        };
      case 'md':
      default:
        return {
          padding: 'var(--space-2) var(--space-4)',
          fontSize: 'var(--text-sm)',
          borderRadius: 'var(--radius-md)',
        };
    }
  };

  const isDisabled = disabled || isLoading;

  const baseStyle: React.CSSProperties = {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 'var(--space-2)',
    fontWeight: 'var(--font-medium)',
    fontFamily: 'var(--font-sans)',
    cursor: isDisabled ? 'not-allowed' : 'pointer',
    opacity: isDisabled ? 0.6 : 1,
    borderWidth: '1px',
    borderStyle: 'solid',
    transition: 'all var(--transition-fast)',
    userSelect: 'none',
    ...getSizeStyles(),
    ...getVariantStyles(),
    ...style,
  };

  return (
    <button
      disabled={isDisabled}
      aria-disabled={isDisabled ? 'true' : undefined}
      aria-busy={isLoading}
      style={baseStyle}
      className={className}
      {...props}
    >
      {isLoading && (
        <span
          role="status"
          aria-label="Loading"
          style={{
            display: 'inline-block',
            width: '1em',
            height: '1em',
            border: '2px solid currentColor',
            borderRightColor: 'transparent',
            borderRadius: 'var(--radius-full)',
            animation: 'spin 0.75s linear infinite',
          }}
        />
      )}
      {children}
    </button>
  );
};
