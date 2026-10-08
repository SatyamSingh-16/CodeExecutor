import { useEffect, useRef, useCallback } from 'react';
import { authStorage } from '../auth/storage';
import { isTerminalStatus } from '../types/submission';
import { getSubmission } from './api';
import type { SubmissionEventPayload } from './types';

export interface UseSubmissionStreamOptions {
  submissionId: string | null;
  onEvent: (event: SubmissionEventPayload) => void;
  onError: (error: Error) => void;
  onClose: () => void;
}

/**
 * Streams real-time submission events via GET /api/submissions/:id/stream.
 *
 * Uses fetch + ReadableStream to transmit the authenticated Bearer token securely
 * without exposing the JWT in URL query strings or weakening backend security.
 *
 * Implements authoritative polling fallback via GET /api/submissions/:id if the
 * stream terminates or errors before receiving a terminal execution state.
 */
export function useSubmissionStream({
  submissionId,
  onEvent,
  onError,
  onClose,
}: UseSubmissionStreamOptions) {
  const abortControllerRef = useRef<AbortController | null>(null);
  const fallbackTimerRef = useRef<number | null>(null);
  const isTerminalRef = useRef<boolean>(false);

  // Stable callback refs to prevent unnecessary reconnects
  const onEventRef = useRef(onEvent);
  const onErrorRef = useRef(onError);
  const onCloseRef = useRef(onClose);

  useEffect(() => {
    onEventRef.current = onEvent;
    onErrorRef.current = onError;
    onCloseRef.current = onClose;
  }, [onEvent, onError, onClose]);

  const cleanup = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    if (fallbackTimerRef.current !== null) {
      window.clearTimeout(fallbackTimerRef.current);
      fallbackTimerRef.current = null;
    }
  }, []);

  const runFallbackPolling = useCallback(
    async (id: string, attempt = 1, maxAttempts = 10) => {
      if (isTerminalRef.current) return;

      try {
        const detail = await getSubmission(id);
        const payload: SubmissionEventPayload = {
          id: detail.id,
          status: detail.status,
          stdout: detail.stdout || '',
          stderr: detail.stderr || '',
          compilation_output: detail.compilation_output || '',
          stdout_truncated: detail.stdout_truncated || false,
          stderr_truncated: detail.stderr_truncated || false,
          exit_code: detail.exit_code,
          execution_time_ms: detail.execution_time_ms,
          memory_usage_kb: detail.memory_usage_kb,
          created_at: detail.created_at,
          updated_at: detail.updated_at,
        };

        onEventRef.current(payload);

        if (isTerminalStatus(detail.status)) {
          isTerminalRef.current = true;
          onCloseRef.current();
          return;
        }

        if (attempt < maxAttempts && !isTerminalRef.current) {
          fallbackTimerRef.current = window.setTimeout(() => {
            void runFallbackPolling(id, attempt + 1, maxAttempts);
          }, 1000);
        } else {
          onErrorRef.current(new Error('Submission timed out waiting for execution result.'));
          onCloseRef.current();
        }
      } catch (err) {
        if (attempt < maxAttempts && !isTerminalRef.current) {
          fallbackTimerRef.current = window.setTimeout(() => {
            void runFallbackPolling(id, attempt + 1, maxAttempts);
          }, 1000);
        } else {
          onErrorRef.current(err instanceof Error ? err : new Error('Failed to retrieve submission state.'));
          onCloseRef.current();
        }
      }
    },
    []
  );

  useEffect(() => {
    if (!submissionId) {
      cleanup();
      return;
    }

    // Reset terminal tracker for new submission
    isTerminalRef.current = false;
    cleanup();

    const controller = new AbortController();
    abortControllerRef.current = controller;

    const connectStream = async () => {
      const token = authStorage.getToken();
      const headers: Record<string, string> = {
        Accept: 'text/event-stream',
      };
      if (token) {
        headers['Authorization'] = `Bearer ${token}`;
      }

      const baseOrigin =
        typeof window !== 'undefined' && window.location?.origin && !window.location.origin.startsWith('null')
          ? window.location.origin
          : 'http://localhost';
      const streamUrl = `${baseOrigin}/api/submissions/${encodeURIComponent(submissionId)}/stream`;

      try {
        const response = await fetch(streamUrl, {
          method: 'GET',
          headers,
          signal: controller.signal,
        });

        if (!response.ok) {
          if (controller.signal.aborted) return;
          console.warn(`[SSE] Stream endpoint returned HTTP ${response.status}, triggering fallback`);
          await runFallbackPolling(submissionId);
          return;
        }

        if (!response.body) {
          if (controller.signal.aborted) return;
          await runFallbackPolling(submissionId);
          return;
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const blocks = buffer.split('\n\n');
          // Retain any incomplete trailing chunk in buffer
          buffer = blocks.pop() ?? '';

          for (const block of blocks) {
            if (!block.trim()) continue;

            const lines = block.split('\n');
            let data = '';

            for (const line of lines) {
              if (line.startsWith(':')) {
                // Heartbeat comment
                continue;
              } else if (line.startsWith('data:')) {
                const text = line.slice(5).trim();
                data = data ? `${data}\n${text}` : text;
              }
            }

            if (data) {
              try {
                const parsed = JSON.parse(data) as SubmissionEventPayload;
                onEventRef.current(parsed);

                if (isTerminalStatus(parsed.status)) {
                  isTerminalRef.current = true;
                  onCloseRef.current();
                  return;
                }
              } catch (parseErr) {
                console.warn('[SSE] Failed to parse event JSON:', parseErr, data);
              }
            }
          }
        }

        // Process any final remaining block after reader ends
        if (buffer.trim()) {
          const lines = buffer.split('\n');
          let data = '';
          for (const line of lines) {
            if (!line.startsWith(':') && line.startsWith('data:')) {
              const text = line.slice(5).trim();
              data = data ? `${data}\n${text}` : text;
            }
          }
          if (data) {
            try {
              const parsed = JSON.parse(data) as SubmissionEventPayload;
              onEventRef.current(parsed);
              if (isTerminalStatus(parsed.status)) {
                isTerminalRef.current = true;
                onCloseRef.current();
                return;
              }
            } catch {
              // Ignore trailing parse errors
            }
          }
        }

        // If reader completed without a terminal status, fall back to DB lookup
        if (!isTerminalRef.current && !controller.signal.aborted) {
          await runFallbackPolling(submissionId);
        }
      } catch (err) {
        if (controller.signal.aborted) return;
        console.warn('[SSE] Connection error, initiating fallback:', err);
        await runFallbackPolling(submissionId);
      }
    };

    void connectStream();

    return () => {
      cleanup();
    };
  }, [submissionId, cleanup, runFallbackPolling]);

  return { abort: cleanup };
}
