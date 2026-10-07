import React from 'react';

export interface StdinPanelProps {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}

export const StdinPanel: React.FC<StdinPanelProps> = ({
  value,
  onChange,
  disabled = false,
}) => {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        backgroundColor: 'var(--bg-surface)',
        borderRadius: 'var(--radius-md)',
        border: '1px solid var(--border-default)',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          padding: 'var(--space-2) var(--space-4)',
          backgroundColor: 'var(--bg-surface-elevated)',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}
      >
        <label
          htmlFor="stdin-input"
          style={{
            fontSize: 'var(--text-xs)',
            fontWeight: 'var(--font-semibold)',
            color: 'var(--text-secondary)',
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
          }}
        >
          Standard Input (stdin)
        </label>
        <span style={{ fontSize: 'var(--text-xs)', color: 'var(--text-muted)' }}>
          Optional
        </span>
      </div>

      <textarea
        id="stdin-input"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        placeholder="Enter custom inputs here (e.g. test cases or CLI arguments)..."
        aria-label="Standard input text area"
        rows={5}
        style={{
          flex: 1,
          width: '100%',
          backgroundColor: 'transparent',
          color: 'var(--text-primary)',
          fontSize: 'var(--text-sm)',
          fontFamily: 'var(--font-mono)',
          padding: 'var(--space-3)',
          border: 'none',
          outline: 'none',
          resize: 'none',
          minHeight: '120px',
        }}
      />
    </div>
  );
};
