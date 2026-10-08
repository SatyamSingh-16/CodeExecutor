import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { HistoryItem } from './HistoryItem';
import type { SubmissionDetail } from '../types/submission';

describe('HistoryItem', () => {
  const mockSubmission: SubmissionDetail = {
    id: '12345678-abcd-1234-abcd-1234567890ab',
    language: 'python',
    source_code: 'print("hello")',
    stdin: '',
    status: 'SUCCESS',
    stdout: 'hello\n',
    stderr: '',
    compilation_output: '',
    stdout_truncated: false,
    stderr_truncated: false,
    exit_code: 0,
    execution_time_ms: 150,
    memory_usage_kb: 1024,
    created_at: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
    updated_at: new Date().toISOString(),
  };

  it('renders status badge, language, and relative timestamp', () => {
    const handleSelect = vi.fn();
    render(<HistoryItem submission={mockSubmission} onSelect={handleSelect} />);

    expect(screen.getByText('SUCCESS')).toBeInTheDocument();
    expect(screen.getByText('Python 3')).toBeInTheDocument();
    expect(screen.getByText(/minutes? ago/i)).toBeInTheDocument();
    expect(screen.getByText(/150 ms · 1.0 MB/)).toBeInTheDocument();
  });

  it('omits metrics when execution_time_ms and memory_usage_kb are absent', () => {
    const withoutMetrics: SubmissionDetail = {
      ...mockSubmission,
      execution_time_ms: null,
      memory_usage_kb: null,
    };
    render(<HistoryItem submission={withoutMetrics} onSelect={vi.fn()} />);

    expect(screen.queryByText(/ms/)).not.toBeInTheDocument();
    expect(screen.queryByText(/MB/)).not.toBeInTheDocument();
  });

  it('triggers onSelect when clicked', () => {
    const handleSelect = vi.fn();
    render(<HistoryItem submission={mockSubmission} onSelect={handleSelect} />);

    const item = screen.getByRole('button');
    fireEvent.click(item);

    expect(handleSelect).toHaveBeenCalledWith(mockSubmission);
  });

  it('triggers onSelect when Enter or Space is pressed', () => {
    const handleSelect = vi.fn();
    render(<HistoryItem submission={mockSubmission} onSelect={handleSelect} />);

    const item = screen.getByRole('button');
    fireEvent.keyDown(item, { key: 'Enter' });
    expect(handleSelect).toHaveBeenCalledTimes(1);

    fireEvent.keyDown(item, { key: ' ' });
    expect(handleSelect).toHaveBeenCalledTimes(2);
  });
});
