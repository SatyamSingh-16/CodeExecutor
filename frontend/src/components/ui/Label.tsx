import React from 'react';

export interface LabelProps extends React.LabelHTMLAttributes<HTMLLabelElement> {
  required?: boolean;
}

export const Label: React.FC<LabelProps> = ({
  children,
  required,
  style,
  ...props
}) => {
  return (
    <label
      style={{
        display: 'inline-block',
        fontSize: 'var(--text-xs)',
        fontWeight: 'var(--font-medium)',
        color: 'var(--text-secondary)',
        marginBottom: 'var(--space-1-5)',
        ...style,
      }}
      {...props}
    >
      {children}
      {required && (
        <span style={{ color: 'var(--color-error)', marginLeft: 'var(--space-1)' }} aria-hidden="true">
          *
        </span>
      )}
    </label>
  );
};
