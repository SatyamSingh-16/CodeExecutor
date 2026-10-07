import React from 'react';
import type { SupportedLanguage } from '../types/submission';

export interface LanguageSelectorProps {
  value: SupportedLanguage;
  onChange: (language: SupportedLanguage) => void;
  disabled?: boolean;
}

export const LanguageSelector: React.FC<LanguageSelectorProps> = ({
  value,
  onChange,
  disabled = false,
}) => {
  return (
    <div style={{ display: 'inline-flex', alignItems: 'center', gap: 'var(--space-2)' }}>
      <label
        htmlFor="language-select"
        style={{
          fontSize: 'var(--text-xs)',
          fontWeight: 'var(--font-medium)',
          color: 'var(--text-secondary)',
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
        }}
      >
        Language
      </label>
      <select
        id="language-select"
        value={value}
        onChange={(e) => onChange(e.target.value as SupportedLanguage)}
        disabled={disabled}
        aria-label="Select Programming Language"
        style={{
          backgroundColor: 'var(--bg-surface-elevated)',
          color: 'var(--text-primary)',
          fontSize: 'var(--text-sm)',
          fontFamily: 'var(--font-mono)',
          padding: 'var(--space-1-5) var(--space-3)',
          borderRadius: 'var(--radius-md)',
          border: '1px solid var(--border-default)',
          outline: 'none',
          cursor: disabled ? 'not-allowed' : 'pointer',
          transition: 'border-color var(--transition-fast), box-shadow var(--transition-fast)',
        }}
      >
        <option value="python">Python 3 (3.11)</option>
        <option value="go">Go (1.22)</option>
      </select>
    </div>
  );
};
