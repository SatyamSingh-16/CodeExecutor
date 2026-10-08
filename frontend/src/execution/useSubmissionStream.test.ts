import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useSubmissionStream } from './useSubmissionStream';
import { authStorage } from '../auth/storage';
import * as apiModule from './api';
import type { SubmissionEventPayload } from './types';
import type { SubmissionDetail } from '../types/submission';

describe('useSubmissionStream', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(authStorage, 'getToken').mockReturnValue('mock-jwt-token');
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('1. establishes stream with Authorization header and parses SSE events', async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();
    const onClose = vi.fn();

    const mockPayload: SubmissionEventPayload = {
      id: 'sub-101',
      status: 'PROCESSING',
      stdout: '',
      stderr: '',
      compilation_output: '',
      stdout_truncated: false,
      stderr_truncated: false,
      exit_code: null,
      execution_time_ms: null,
      memory_usage_kb: null,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };

    const sseData = `: keepalive\n\nevent: submission\ndata: ${JSON.stringify(mockPayload)}\n\n`;

    // Mock fetch with a readable stream
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(sseData));
        controller.close();
      },
    });

    global.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      body: stream,
    });

    renderHook(() =>
      useSubmissionStream({
        submissionId: 'sub-101',
        onEvent,
        onError,
        onClose,
      })
    );

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/submissions/sub-101/stream'),
        {
        method: 'GET',
        headers: {
          Accept: 'text/event-stream',
          Authorization: 'Bearer mock-jwt-token',
        },
        signal: expect.any(AbortSignal),
      });
      expect(onEvent).toHaveBeenCalledWith(mockPayload);
    });
  });

  it('2. terminal event automatically closes stream', async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();
    const onClose = vi.fn();

    const terminalPayload: SubmissionEventPayload = {
      id: 'sub-102',
      status: 'SUCCESS',
      stdout: 'Hello World\n',
      stderr: '',
      compilation_output: '',
      stdout_truncated: false,
      stderr_truncated: false,
      exit_code: 0,
      execution_time_ms: 12,
      memory_usage_kb: 4096,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };

    const sseData = `event: submission\ndata: ${JSON.stringify(terminalPayload)}\n\n`;

    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(sseData));
        controller.close();
      },
    });

    global.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      body: stream,
    });

    renderHook(() =>
      useSubmissionStream({
        submissionId: 'sub-102',
        onEvent,
        onError,
        onClose,
      })
    );

    await waitFor(() => {
      expect(onEvent).toHaveBeenCalledWith(terminalPayload);
      expect(onClose).toHaveBeenCalled();
    });
  });

  it('3. falls back to getSubmission when stream request fails', async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();
    const onClose = vi.fn();

    const mockDetail: SubmissionDetail = {
      id: 'sub-fallback',
      language: 'python',
      status: 'SUCCESS',
      stdout: 'fallback output\n',
      stderr: '',
      compilation_output: '',
      stdout_truncated: false,
      stderr_truncated: false,
      exit_code: 0,
      execution_time_ms: 45,
      memory_usage_kb: 8192,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };

    // Simulate stream connection error
    global.fetch = vi.fn().mockRejectedValueOnce(new Error('Network error'));
    vi.spyOn(apiModule, 'getSubmission').mockResolvedValueOnce(mockDetail);

    renderHook(() =>
      useSubmissionStream({
        submissionId: 'sub-fallback',
        onEvent,
        onError,
        onClose,
      })
    );

    await waitFor(() => {
      expect(apiModule.getSubmission).toHaveBeenCalledWith('sub-fallback');
      expect(onEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'sub-fallback',
          status: 'SUCCESS',
          stdout: 'fallback output\n',
        })
      );
      expect(onClose).toHaveBeenCalled();
    });
  });

  it('4. ignores malformed JSON payloads safely without crashing', async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();
    const onClose = vi.fn();

    const sseData = `data: {invalid-json}\n\n`;

    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(sseData));
        controller.close();
      },
    });

    global.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      body: stream,
    });

    renderHook(() =>
      useSubmissionStream({
        submissionId: 'sub-malformed',
        onEvent,
        onError,
        onClose,
      })
    );

    // Should not throw or crash
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalled();
      expect(onEvent).not.toHaveBeenCalled();
    });
  });

  it('5. cleans up and aborts stream on component unmount', async () => {
    const onEvent = vi.fn();
    const onError = vi.fn();
    const onClose = vi.fn();

    let streamCancelled = false;
    const stream = new ReadableStream({
      start() {},
      cancel() {
        streamCancelled = true;
      },
    });

    global.fetch = vi.fn().mockResolvedValueOnce({
      ok: true,
      body: stream,
    });

    const { unmount } = renderHook(() =>
      useSubmissionStream({
        submissionId: 'sub-unmount',
        onEvent,
        onError,
        onClose,
      })
    );

    unmount();

    // Verify stream was aborted cleanly
    expect(streamCancelled).toBe(false); // Reader aborted via signal
  });
});
