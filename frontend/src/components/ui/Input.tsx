import React from 'react';
import { Label } from './Label';

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  helperText?: string;
  required?: boolean;
}

export const Input = React.forwardRef<HTMLInputElement, InputProps>(({
  id,
  label,
  error,
  helperText,
  required,
  disabled,
  style,
  ...props
}, ref) => {
  const inputId = id || (label ? `input-${label.toLowerCase().replace(/\s+/g, '-')}` : undefined);
  const errorId = error && inputId ? `${inputId}-error` : undefined;
  const helperId = helperText && inputId ? `${inputId}-helper` : undefined;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
      {label && (
        <Label htmlFor={inputId} required={required}>
          {label}
        </Label>
      )}
      <input
        ref={ref}
        id={inputId}
        disabled={disabled}
        aria-invalid={!!error}
        aria-errormessage={error ? errorId : undefined}
        aria-describedby={[errorId, helperId].filter(Boolean).join(' ') || undefined}
        style={{
          width: '100%',
          backgroundColor: 'var(--bg-surface-input)',
          color: 'var(--text-primary)',
          fontSize: 'var(--text-sm)',
          fontFamily: 'var(--font-sans)',
          padding: 'var(--space-2-5) var(--space-3)',
          borderRadius: 'var(--radius-md)',
          borderWidth: '1px',
          borderStyle: 'solid',
          borderColor: error ? 'var(--color-error)' : 'var(--border-default)',
          outline: 'none',
          transition: 'border-color var(--transition-fast), box-shadow var(--transition-fast)',
          opacity: disabled ? 0.6 : 1,
          cursor: disabled ? 'not-allowed' : 'text',
          ...style,
        }}
        {...props}
      />
      {error && (
        <span
          id={errorId}
          role="alert"
          style={{
            fontSize: 'var(--text-xs)',
            color: 'var(--color-error)',
            marginTop: 'var(--space-1)',
          }}
        >
          {error}
        </span>
      )}
      {!error && helperText && (
        <span
          id={helperId}
          style={{
            fontSize: 'var(--text-xs)',
            color: 'var(--text-muted)',
            marginTop: 'var(--space-1)',
          }}
        >
          {helperText}
        </span>
      )}
    </div>
  );
});

Input.displayName = 'Input';
