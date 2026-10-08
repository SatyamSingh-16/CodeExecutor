import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useExecution } from './useExecution';
import * as apiModule from './api';

describe('useExecution', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('1. initializes in idle state with no active submission', () => {
    const { result } = renderHook(() => useExecution());

    expect(result.current.status).toBe('idle');
    expect(result.current.isRunning).toBe(false);
    expect(result.current.submissionId).toBeNull();
    expect(result.current.submission).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it('2. transitions from idle -> submitting -> queued on submitCode success', async () => {
    vi.spyOn(apiModule, 'submitCode').mockResolvedValueOnce({
      id: 'sub-new-1',
      status: 'QUEUED',
    });

    const { result } = renderHook(() => useExecution());

    await act(async () => {
      await result.current.submit({
        language: 'python',
        sourceCode: 'print(1)',
        stdin: '',
      });
    });

    expect(result.current.submissionId).toBe('sub-new-1');
    expect(result.current.status).toBe('queued');
    expect(result.current.isRunning).toBe(true);
  });

  it('3. prevents duplicate submission while isRunning is true', async () => {
    const submitSpy = vi.spyOn(apiModule, 'submitCode').mockResolvedValue({
      id: 'sub-dup-1',
      status: 'QUEUED',
    });

    const { result } = renderHook(() => useExecution());

    await act(async () => {
      await result.current.submit({
        language: 'python',
        sourceCode: 'print(1)',
      });
    });

    expect(result.current.isRunning).toBe(true);
    expect(submitSpy).toHaveBeenCalledTimes(1);

    // Attempt second submission while running
    await act(async () => {
      await result.current.submit({
        language: 'python',
        sourceCode: 'print(2)',
      });
    });

    // Should NOT call submitCode again
    expect(submitSpy).toHaveBeenCalledTimes(1);
  });

  it('4. rejects empty source code without calling API', async () => {
    const submitSpy = vi.spyOn(apiModule, 'submitCode');
    const { result } = renderHook(() => useExecution());

    await act(async () => {
      await result.current.submit({
        language: 'python',
        sourceCode: '   ',
      });
    });

    expect(submitSpy).not.toHaveBeenCalled();
    expect(result.current.status).toBe('error');
    expect(result.current.error).toBe('Cannot execute empty source code.');
    expect(result.current.isRunning).toBe(false);
  });

  it('5. handles API submission error and restores idle/error state without fake result', async () => {
    vi.spyOn(apiModule, 'submitCode').mockRejectedValueOnce(
      new Error('Rate limit exceeded')
    );

    const { result } = renderHook(() => useExecution());

    await act(async () => {
      await result.current.submit({
        language: 'python',
        sourceCode: 'print("hello")',
      });
    });

    expect(result.current.status).toBe('error');
    expect(result.current.error).toBe('Rate limit exceeded');
    expect(result.current.submission).toBeNull();
    expect(result.current.isRunning).toBe(false);
  });

  it('6. reset restores initial idle state', async () => {
    vi.spyOn(apiModule, 'submitCode').mockResolvedValueOnce({
      id: 'sub-reset',
      status: 'QUEUED',
    });

    const { result } = renderHook(() => useExecution());

    await act(async () => {
      await result.current.submit({
        language: 'go',
        sourceCode: 'package main',
      });
    });

    expect(result.current.status).toBe('queued');

    act(() => {
      result.current.reset();
    });

    expect(result.current.status).toBe('idle');
    expect(result.current.submissionId).toBeNull();
    expect(result.current.submission).toBeNull();
    expect(result.current.isRunning).toBe(false);
  });
});
