import React from 'react';

export interface PageContainerProps extends React.HTMLAttributes<HTMLDivElement> {
  maxWidth?: 'sm' | 'md' | 'lg' | 'xl' | 'full';
}

export const PageContainer: React.FC<PageContainerProps> = ({
  children,
  maxWidth = 'xl',
  style,
  className = '',
  ...props
}) => {
  const getMaxWidth = () => {
    switch (maxWidth) {
      case 'sm':
        return 'var(--container-sm)';
      case 'md':
        return 'var(--container-md)';
      case 'lg':
        return 'var(--container-lg)';
      case 'full':
        return '100%';
      case 'xl':
      default:
        return 'var(--container-xl)';
    }
  };

  return (
    <div
      style={{
        width: '100%',
        maxWidth: getMaxWidth(),
        margin: '0 auto',
        padding: 'var(--space-6) var(--space-4)',
        ...style,
      }}
      className={className}
      {...props}
    >
      {children}
    </div>
  );
};
