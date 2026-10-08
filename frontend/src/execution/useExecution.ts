import { useState, useCallback, useMemo } from 'react';
import type { SupportedLanguage } from '../types/submission';
import { isTerminalStatus } from '../types/submission';
import { submitCode } from './api';
import { useSubmissionStream } from './useSubmissionStream';
import type { ExecutionState, ExecutionStatus, SubmissionEventPayload } from './types';

export interface SubmitOptions {
  language: SupportedLanguage;
  sourceCode: string;
  stdin?: string;
}

export function useExecution() {
  const [status, setStatus] = useState<ExecutionStatus>('idle');
  const [submissionId, setSubmissionId] = useState<string | null>(null);
  const [submission, setSubmission] = useState<SubmissionEventPayload | null>(null);
  const [error, setError] = useState<string | null>(null);

  const isRunning = useMemo(() => {
    return status === 'submitting' || status === 'queued' || status === 'processing';
  }, [status]);

  const handleStreamEvent = useCallback((event: SubmissionEventPayload) => {
    setSubmission(event);
    if (event.status === 'QUEUED') {
      setStatus('queued');
    } else if (event.status === 'PROCESSING') {
      setStatus('processing');
    } else if (isTerminalStatus(event.status)) {
      setStatus('terminal');
    }
  }, []);

  const handleStreamError = useCallback((err: Error) => {
    setError(err.message || 'Stream connection error');
    setStatus('error');
  }, []);

  const handleStreamClose = useCallback(() => {
    setSubmissionId(null);
  }, []);

  const { abort } = useSubmissionStream({
    submissionId,
    onEvent: handleStreamEvent,
    onError: handleStreamError,
    onClose: handleStreamClose,
  });

  const submit = useCallback(
    async ({ language, sourceCode, stdin }: SubmitOptions) => {
      if (isRunning) {
        return;
      }

      if (!sourceCode.trim()) {
        setError('Cannot execute empty source code.');
        setStatus('error');
        return;
      }

      // Abort any lingering stream from earlier execution
      abort();
      setSubmissionId(null);
      setError(null);
      setSubmission(null);
      setStatus('submitting');

      try {
        const response = await submitCode({
          language,
          source_code: sourceCode,
          stdin: stdin || '',
        });

        setStatus('queued');
        setSubmissionId(response.id);
      } catch (err) {
        setStatus('error');
        setError(err instanceof Error ? err.message : 'Submission failed');
      }
    },
    [isRunning, abort]
  );

  const reset = useCallback(() => {
    abort();
    setSubmissionId(null);
    setSubmission(null);
    setError(null);
    setStatus('idle');
  }, [abort]);

  const state: ExecutionState = useMemo(
    () => ({
      status,
      submissionId,
      submission,
      error,
      isRunning,
    }),
    [status, submissionId, submission, error, isRunning]
  );

  return {
    state,
    status,
    submission,
    submissionId,
    error,
    isRunning,
    submit,
    reset,
  };
}
