import React from 'react';

export interface LoadingSpinnerProps {
  size?: 'sm' | 'md' | 'lg';
  color?: string;
  label?: string;
}

export const LoadingSpinner: React.FC<LoadingSpinnerProps> = ({
  size = 'md',
  color = 'var(--accent-primary)',
  label = 'Loading...',
}) => {
  const getDimension = () => {
    switch (size) {
      case 'sm':
        return '16px';
      case 'lg':
        return '36px';
      case 'md':
      default:
        return '24px';
    }
  };

  const dim = getDimension();

  return (
    <div
      role="status"
      aria-label={label}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <span
        style={{
          width: dim,
          height: dim,
          borderRadius: 'var(--radius-full)',
          border: '2px solid transparent',
          borderTopColor: color,
          borderRightColor: color,
          display: 'inline-block',
          animation: 'spin 0.6s linear infinite',
        }}
      />
      <span style={{ position: 'absolute', width: '1px', height: '1px', overflow: 'hidden', clip: 'rect(0,0,0,0)' }}>
        {label}
      </span>
      <style>
        {`
          @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
          }
        `}
      </style>
    </div>
  );
};
