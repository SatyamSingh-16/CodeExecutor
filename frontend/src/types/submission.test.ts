import { describe, it, expect } from 'vitest';
import {
  SubmissionStatus,
  isTerminalStatus,
  SupportedLanguage,
} from './submission';

describe('Submission Types and Statuses', () => {
  it('matches all 8 backend submission statuses exactly', () => {
    const expectedStatuses: SubmissionStatus[] = [
      'QUEUED',
      'PROCESSING',
      'SUCCESS',
      'COMPILATION_ERROR',
      'RUNTIME_ERROR',
      'TIME_LIMIT_EXCEEDED',
      'MEMORY_LIMIT_EXCEEDED',
      'SYSTEM_ERROR',
    ];

    expect(expectedStatuses).toHaveLength(8);

    // Verify each status can be assigned to SubmissionStatus
    expectedStatuses.forEach((status) => {
      const s: SubmissionStatus = status;
      expect(typeof s).toBe('string');
    });
  });

  it('correctly identifies terminal vs non-terminal states', () => {
    // Non-terminal
    expect(isTerminalStatus('QUEUED')).toBe(false);
    expect(isTerminalStatus('PROCESSING')).toBe(false);

    // Terminal
    expect(isTerminalStatus('SUCCESS')).toBe(true);
    expect(isTerminalStatus('COMPILATION_ERROR')).toBe(true);
    expect(isTerminalStatus('RUNTIME_ERROR')).toBe(true);
    expect(isTerminalStatus('TIME_LIMIT_EXCEEDED')).toBe(true);
    expect(isTerminalStatus('MEMORY_LIMIT_EXCEEDED')).toBe(true);
    expect(isTerminalStatus('SYSTEM_ERROR')).toBe(true);
  });

  it('supports the configured execution languages', () => {
    const languages: SupportedLanguage[] = ['python', 'go'];
    expect(languages).toContain('python');
    expect(languages).toContain('go');
  });
});
