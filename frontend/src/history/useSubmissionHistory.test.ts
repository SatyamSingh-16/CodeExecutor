import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useSubmissionHistory } from './useSubmissionHistory';
import * as api from './api';
import type { SubmissionDetail } from '../types/submission';

vi.mock('./api', () => ({
  PAGE_SIZE: 20,
  listSubmissions: vi.fn(),
  getSubmission: vi.fn(),
}));

describe('useSubmissionHistory', () => {
  const mockSubmissions: SubmissionDetail[] = [
    {
      id: 'sub-1',
      language: 'python',
      source_code: 'print("1")',
      stdin: '',
      status: 'SUCCESS',
      stdout: '1\n',
      stderr: '',
      compilation_output: '',
      stdout_truncated: false,
      stderr_truncated: false,
      exit_code: 0,
      execution_time_ms: 120,
      memory_usage_kb: 1024,
      created_at: '2026-10-08T10:00:00Z',
      updated_at: '2026-10-08T10:00:01Z',
    },
    {
      id: 'sub-2',
      language: 'go',
      source_code: 'package main',
      stdin: '',
      status: 'RUNTIME_ERROR',
      stdout: '',
      stderr: 'runtime error',
      compilation_output: '',
      stdout_truncated: false,
      stderr_truncated: false,
      exit_code: 1,
      execution_time_ms: null,
      memory_usage_kb: null,
      created_at: '2026-10-08T09:00:00Z',
      updated_at: '2026-10-08T09:00:01Z',
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('1. initializes in closed state with empty submissions', () => {
    const { result } = renderHook(() => useSubmissionHistory());

    expect(result.current.isOpen).toBe(false);
    expect(result.current.submissions).toEqual([]);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBeNull();
    expect(result.current.selectedSubmission).toBeNull();
  });

  it('2. openDrawer opens drawer and loads page 1', async () => {
    (api.listSubmissions as any).mockResolvedValueOnce(mockSubmissions);

    const { result } = renderHook(() => useSubmissionHistory());

    await act(async () => {
      result.current.openDrawer();
    });

    expect(result.current.isOpen).toBe(true);
    expect(api.listSubmissions).toHaveBeenCalledWith(1, 20);
    expect(result.current.submissions).toEqual(mockSubmissions);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('3. closeDrawer closes drawer and clears selection', async () => {
    (api.listSubmissions as any).mockResolvedValueOnce(mockSubmissions);
    (api.getSubmission as any).mockResolvedValueOnce(mockSubmissions[0]);

    const { result } = renderHook(() => useSubmissionHistory());

    await act(async () => {
      result.current.openDrawer();
    });

    await act(async () => {
      await result.current.selectSubmission('sub-1');
    });

    expect(result.current.selectedSubmission).toEqual(mockSubmissions[0]);

    act(() => {
      result.current.closeDrawer();
    });

    expect(result.current.isOpen).toBe(false);
    expect(result.current.selectedSubmission).toBeNull();
    expect(result.current.selectedId).toBeNull();
  });

  it('4. handles list fetch errors gracefully', async () => {
    (api.listSubmissions as any).mockRejectedValueOnce(new Error('Failed to load'));

    const { result } = renderHook(() => useSubmissionHistory());

    await act(async () => {
      result.current.openDrawer();
    });

    expect(result.current.error).toBe('Failed to load');
    expect(result.current.submissions).toEqual([]);
    expect(result.current.isLoading).toBe(false);
  });

  it('5. loadMore fetches next page and appends submissions', async () => {
    // 20 items so hasMore remains true
    const page1 = Array.from({ length: 20 }, (_, i) => ({
      ...mockSubmissions[0],
      id: `sub-page1-${i}`,
    }));
    const page2 = [mockSubmissions[1]];

    (api.listSubmissions as any)
      .mockResolvedValueOnce(page1)
      .mockResolvedValueOnce(page2);

    const { result } = renderHook(() => useSubmissionHistory());

    await act(async () => {
      result.current.openDrawer();
    });

    expect(result.current.submissions).toHaveLength(20);
    expect(result.current.hasMore).toBe(true);

    await act(async () => {
      result.current.loadMore();
    });

    expect(api.listSubmissions).toHaveBeenCalledWith(2, 20);
    expect(result.current.submissions).toHaveLength(21);
    expect(result.current.hasMore).toBe(false);
  });

  it('6. selectSubmission retrieves authoritative details from api', async () => {
    (api.getSubmission as any).mockResolvedValueOnce(mockSubmissions[0]);

    const { result } = renderHook(() => useSubmissionHistory());

    await act(async () => {
      await result.current.selectSubmission('sub-1');
    });

    expect(api.getSubmission).toHaveBeenCalledWith('sub-1');
    expect(result.current.selectedSubmission).toEqual(mockSubmissions[0]);
    expect(result.current.isLoadingDetail).toBe(false);
    expect(result.current.detailError).toBeNull();
  });

  it('7. selectSubmission captures detail retrieval errors', async () => {
    (api.getSubmission as any).mockRejectedValueOnce(new Error('Submission not found'));

    const { result } = renderHook(() => useSubmissionHistory());

    await act(async () => {
      await result.current.selectSubmission('sub-404');
    });

    expect(result.current.detailError).toBe('Submission not found');
    expect(result.current.selectedSubmission).toBeNull();
    expect(result.current.isLoadingDetail).toBe(false);
  });

  it('8. clearSelection resets selected item and errors', async () => {
    (api.getSubmission as any).mockResolvedValueOnce(mockSubmissions[0]);

    const { result } = renderHook(() => useSubmissionHistory());

    await act(async () => {
      await result.current.selectSubmission('sub-1');
    });

    expect(result.current.selectedSubmission).not.toBeNull();

    act(() => {
      result.current.clearSelection();
    });

    expect(result.current.selectedSubmission).toBeNull();
    expect(result.current.selectedId).toBeNull();
    expect(result.current.detailError).toBeNull();
  });

  it('9. refresh reloads page 1 cleanly', async () => {
    (api.listSubmissions as any).mockResolvedValueOnce(mockSubmissions);

    const { result } = renderHook(() => useSubmissionHistory());

    await act(async () => {
      await result.current.refresh();
    });

    expect(api.listSubmissions).toHaveBeenCalledWith(1, 20);
    expect(result.current.submissions).toEqual(mockSubmissions);
  });
});
