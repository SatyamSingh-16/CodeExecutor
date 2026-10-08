import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { HistoryDrawer, type HistoryDrawerProps } from './HistoryDrawer';
import type { SubmissionDetail } from '../types/submission';

describe('HistoryDrawer', () => {
  const mockSubmissions: SubmissionDetail[] = [
    {
      id: 'sub-item-1',
      language: 'python',
      source_code: 'print("first")',
      stdin: 'input-1',
      status: 'SUCCESS',
      stdout: 'first\n',
      stderr: '',
      compilation_output: '',
      stdout_truncated: false,
      stderr_truncated: false,
      exit_code: 0,
      execution_time_ms: 120,
      memory_usage_kb: 4096,
      created_at: new Date(Date.now() - 60000).toISOString(),
      updated_at: new Date().toISOString(),
    },
    {
      id: 'sub-item-2',
      language: 'go',
      source_code: 'package main\n\nfunc main() {}',
      stdin: '',
      status: 'RUNTIME_ERROR',
      stdout: '',
      stderr: 'panic: test error',
      compilation_output: '',
      stdout_truncated: false,
      stderr_truncated: false,
      exit_code: 2,
      execution_time_ms: 80,
      memory_usage_kb: 2048,
      created_at: new Date(Date.now() - 3600000).toISOString(),
      updated_at: new Date().toISOString(),
    },
  ];

  const defaultProps: HistoryDrawerProps = {
    isOpen: true,
    onClose: vi.fn(),
    submissions: mockSubmissions,
    isLoading: false,
    error: null,
    hasMore: false,
    onLoadMore: vi.fn(),
    onRefresh: vi.fn(),
    selectedId: null,
    selectedSubmission: null,
    isLoadingDetail: false,
    detailError: null,
    onSelectSubmission: vi.fn(),
    onClearSelection: vi.fn(),
    onRestore: vi.fn(),
    isWorkspaceModified: false,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('1. returns null when isOpen is false', () => {
    const { container } = render(<HistoryDrawer {...defaultProps} isOpen={false} />);
    expect(container.firstChild).toBeNull();
  });

  it('2. renders drawer dialog and close button when open', () => {
    render(<HistoryDrawer {...defaultProps} />);

    expect(screen.getByRole('dialog', { name: /submission history drawer/i })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /submission history/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /close history/i })).toBeInTheDocument();
  });

  it('3. closes drawer when Close button is clicked', () => {
    const handleClose = vi.fn();
    render(<HistoryDrawer {...defaultProps} onClose={handleClose} />);

    fireEvent.click(screen.getByRole('button', { name: /close history/i }));
    expect(handleClose).toHaveBeenCalledTimes(1);
  });

  it('4. closes drawer on Escape key press', () => {
    const handleClose = vi.fn();
    render(<HistoryDrawer {...defaultProps} onClose={handleClose} />);

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(handleClose).toHaveBeenCalledTimes(1);
  });

  it('5. renders loading state while initial history is being fetched', () => {
    render(<HistoryDrawer {...defaultProps} submissions={[]} isLoading={true} />);

    expect(screen.getByText(/loading past submissions\.\.\./i)).toBeInTheDocument();
  });

  it('6. renders empty state when user has zero submissions', () => {
    render(<HistoryDrawer {...defaultProps} submissions={[]} isLoading={false} />);

    expect(screen.getByText(/no submissions yet/i)).toBeInTheDocument();
    expect(screen.getByText(/submitted programs and their execution metrics will appear here/i)).toBeInTheDocument();
  });

  it('7. renders error state and triggers onRefresh on retry click', () => {
    const handleRefresh = vi.fn();
    render(<HistoryDrawer {...defaultProps} submissions={[]} error="Failed to fetch history" onRefresh={handleRefresh} />);

    expect(screen.getByText(/failed to fetch history/i)).toBeInTheDocument();
    const retryBtn = screen.getByRole('button', { name: /retry/i });
    fireEvent.click(retryBtn);

    expect(handleRefresh).toHaveBeenCalledTimes(1);
  });

  it('8. renders submission items with status, language, and relative time', () => {
    render(<HistoryDrawer {...defaultProps} />);

    expect(screen.getByText('SUCCESS')).toBeInTheDocument();
    expect(screen.getByText('RUNTIME_ERROR')).toBeInTheDocument();
    expect(screen.getByText('Python 3')).toBeInTheDocument();
    expect(screen.getByText('Go')).toBeInTheDocument();
  });

  it('9. selecting an item invokes onSelectSubmission with submission ID', () => {
    const handleSelect = vi.fn();
    render(<HistoryDrawer {...defaultProps} onSelectSubmission={handleSelect} />);

    const firstItem = screen.getByLabelText(/submission sub-item, SUCCESS, Python 3/i);
    fireEvent.click(firstItem);

    expect(handleSelect).toHaveBeenCalledWith('sub-item-1');
  });

  it('10. renders Load More button when hasMore is true and triggers onLoadMore', () => {
    const handleLoadMore = vi.fn();
    render(<HistoryDrawer {...defaultProps} hasMore={true} onLoadMore={handleLoadMore} />);

    const loadMoreBtn = screen.getByRole('button', { name: /load more/i });
    expect(loadMoreBtn).toBeInTheDocument();

    fireEvent.click(loadMoreBtn);
    expect(handleLoadMore).toHaveBeenCalledTimes(1);
  });

  it('11. renders detail loading state when inspecting an item', () => {
    render(<HistoryDrawer {...defaultProps} selectedId="sub-item-1" isLoadingDetail={true} />);

    expect(screen.getByText(/loading submission details\.\.\./i)).toBeInTheDocument();
  });

  it('12. renders detail error state and retry action', () => {
    const handleSelect = vi.fn();
    render(
      <HistoryDrawer
        {...defaultProps}
        selectedId="sub-item-1"
        detailError="Could not load detail"
        onSelectSubmission={handleSelect}
      />
    );

    expect(screen.getByText(/could not load detail/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /retry/i }));
    expect(handleSelect).toHaveBeenCalledWith('sub-item-1');
  });

  it('13. inspecting detail renders historical output tabs and back button', () => {
    const handleClear = vi.fn();
    render(
      <HistoryDrawer
        {...defaultProps}
        selectedId="sub-item-1"
        selectedSubmission={mockSubmissions[0]}
        onClearSelection={handleClear}
      />
    );

    expect(screen.getByText(/submission detail/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /back to history list/i })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /back to history list/i }));
    expect(handleClear).toHaveBeenCalledTimes(1);

    // Stdout content is visible
    expect(screen.getByText('first')).toBeInTheDocument();
  });

  it('14. switching tabs in detail view displays stderr and source code', () => {
    render(
      <HistoryDrawer
        {...defaultProps}
        selectedId="sub-item-1"
        selectedSubmission={mockSubmissions[0]}
      />
    );

    // Switch to code tab
    fireEvent.click(screen.getByRole('tab', { name: /source code/i }));
    expect(screen.getByText('print("first")')).toBeInTheDocument();
    expect(screen.getByText('input-1')).toBeInTheDocument();
  });

  it('15. displays output truncation alert when output is truncated', () => {
    const truncatedSub: SubmissionDetail = {
      ...mockSubmissions[0],
      stdout_truncated: true,
    };

    render(
      <HistoryDrawer
        {...defaultProps}
        selectedId="sub-item-1"
        selectedSubmission={truncatedSub}
      />
    );

    expect(screen.getByText(/output truncated at 64 kb limit/i)).toBeInTheDocument();
  });

  it('16. Load into Editor immediately restores when workspace is unmodified', () => {
    const handleRestore = vi.fn();
    const handleClose = vi.fn();

    render(
      <HistoryDrawer
        {...defaultProps}
        selectedId="sub-item-1"
        selectedSubmission={mockSubmissions[0]}
        onRestore={handleRestore}
        onClose={handleClose}
        isWorkspaceModified={false}
      />
    );

    const restoreBtn = screen.getByRole('button', { name: /load into editor/i });
    fireEvent.click(restoreBtn);

    expect(handleRestore).toHaveBeenCalledWith({
      language: 'python',
      sourceCode: 'print("first")',
      stdin: 'input-1',
    });
    expect(handleClose).toHaveBeenCalledTimes(1);
  });

  it('17. Load into Editor prompts confirmation when workspace is modified', () => {
    const handleRestore = vi.fn();

    render(
      <HistoryDrawer
        {...defaultProps}
        selectedId="sub-item-1"
        selectedSubmission={mockSubmissions[0]}
        onRestore={handleRestore}
        isWorkspaceModified={true}
      />
    );

    const restoreBtn = screen.getByRole('button', { name: /load into editor/i });
    fireEvent.click(restoreBtn);

    // Confirmation message displayed, restore not yet called
    expect(screen.getByText(/replace current editor contents with this submission\?/i)).toBeInTheDocument();
    expect(handleRestore).not.toHaveBeenCalled();

    // Cancel preserves workspace
    const cancelBtn = screen.getByRole('button', { name: /cancel/i });
    fireEvent.click(cancelBtn);

    expect(screen.queryByText(/replace current editor contents with this submission\?/i)).not.toBeInTheDocument();
    expect(handleRestore).not.toHaveBeenCalled();
  });

  it('18. Confirming replacement in confirmation modal restores workspace', () => {
    const handleRestore = vi.fn();
    const handleClose = vi.fn();

    render(
      <HistoryDrawer
        {...defaultProps}
        selectedId="sub-item-1"
        selectedSubmission={mockSubmissions[0]}
        onRestore={handleRestore}
        onClose={handleClose}
        isWorkspaceModified={true}
      />
    );

    // First click to open confirmation
    fireEvent.click(screen.getByRole('button', { name: /load into editor/i }));

    // Second click on confirm button inside confirmation box
    const confirmBtn = screen.getByRole('button', { name: /load into editor/i });
    fireEvent.click(confirmBtn);

    expect(handleRestore).toHaveBeenCalledWith({
      language: 'python',
      sourceCode: 'print("first")',
      stdin: 'input-1',
    });
    expect(handleClose).toHaveBeenCalledTimes(1);
  });
});
