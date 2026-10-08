import React from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ResultConsole } from './ResultConsole';
import type { ExecutionState } from './types';

describe('ResultConsole', () => {
  it('1. renders idle state when no submission has been initiated', () => {
    const idleState: ExecutionState = {
      status: 'idle',
      submissionId: null,
      submission: null,
      error: null,
      isRunning: false,
    };

    render(<ResultConsole state={idleState} />);

    expect(screen.getByText(/execution output/i)).toBeInTheDocument();
    expect(screen.getByText(/ready to execute/i)).toBeInTheDocument();
    expect(screen.getByText('READY')).toBeInTheDocument();
  });

  it('2. renders QUEUED state waiting indicator', () => {
    const queuedState: ExecutionState = {
      status: 'queued',
      submissionId: 'sub-queued-1',
      submission: null,
      error: null,
      isRunning: true,
    };

    render(<ResultConsole state={queuedState} />);

    expect(screen.getByText('QUEUED')).toBeInTheDocument();
    expect(screen.getByText(/job queued\. waiting for an available worker/i)).toBeInTheDocument();
  });

  it('3. renders PROCESSING state container sandbox indicator', () => {
    const processingState: ExecutionState = {
      status: 'processing',
      submissionId: 'sub-proc-1',
      submission: null,
      error: null,
      isRunning: true,
    };

    render(<ResultConsole state={processingState} />);

    expect(screen.getByText('PROCESSING')).toBeInTheDocument();
    expect(screen.getByText(/executing inside isolated container sandbox/i)).toBeInTheDocument();
  });

  it('4. renders SUCCESS state with stdout and benchmark metrics', () => {
    const successState: ExecutionState = {
      status: 'terminal',
      submissionId: 'sub-success-1',
      submission: {
        id: 'sub-success-1',
        status: 'SUCCESS',
        stdout: 'Hello World!\nProgram complete.\n',
        stderr: '',
        compilation_output: '',
        stdout_truncated: false,
        stderr_truncated: false,
        exit_code: 0,
        execution_time_ms: 42,
        memory_usage_kb: 14336, // 14 MB
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      error: null,
      isRunning: false,
    };

    render(<ResultConsole state={successState} />);

    expect(screen.getByText('SUCCESS')).toBeInTheDocument();
    const stdoutPre = screen.getByLabelText(/stdout console output/i);
    expect(stdoutPre).toHaveTextContent(/hello world!/i);
    expect(stdoutPre).toHaveTextContent(/program complete\./i);
    expect(screen.getByText('42 ms')).toBeInTheDocument();
    expect(screen.getByText('14.0 MB')).toBeInTheDocument();
    expect(screen.getByText('0')).toBeInTheDocument();
  });

  it('5. renders stderr separately in the Standard Error tab', () => {
    const stderrState: ExecutionState = {
      status: 'terminal',
      submissionId: 'sub-err-1',
      submission: {
        id: 'sub-err-1',
        status: 'RUNTIME_ERROR',
        stdout: 'Partial standard output',
        stderr: 'Traceback (most recent call last):\nValueError: invalid input\n',
        compilation_output: '',
        stdout_truncated: false,
        stderr_truncated: false,
        exit_code: 1,
        execution_time_ms: 15,
        memory_usage_kb: 4096,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      error: null,
      isRunning: false,
    };

    render(<ResultConsole state={stderrState} />);

    // Automatically switched to Standard Error tab due to RUNTIME_ERROR
    expect(screen.getByText(/traceback \(most recent call last\)/i)).toBeInTheDocument();

    // Click stdout tab
    const stdoutTab = screen.getByRole('tab', { name: /standard output/i });
    fireEvent.click(stdoutTab);

    expect(screen.getByText(/partial standard output/i)).toBeInTheDocument();
  });

  it('6. auto-selects Compilation tab on COMPILATION_ERROR and displays compilation output', () => {
    const compilationErrorState: ExecutionState = {
      status: 'terminal',
      submissionId: 'sub-comp-1',
      submission: {
        id: 'sub-comp-1',
        status: 'COMPILATION_ERROR',
        stdout: '',
        stderr: '',
        compilation_output: './main.go:4:2: undefined: fmt.Printl\n',
        stdout_truncated: false,
        stderr_truncated: false,
        exit_code: 2,
        execution_time_ms: 150,
        memory_usage_kb: 8192,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      error: null,
      isRunning: false,
    };

    render(<ResultConsole state={compilationErrorState} />);

    expect(screen.getByText('COMPILATION_ERROR')).toBeInTheDocument();
    expect(screen.getByText(/\.\/main\.go:4:2: undefined: fmt\.Printl/i)).toBeInTheDocument();
  });

  it('7. displays truncation warning when stdout_truncated is true', () => {
    const truncatedState: ExecutionState = {
      status: 'terminal',
      submissionId: 'sub-trunc-1',
      submission: {
        id: 'sub-trunc-1',
        status: 'SUCCESS',
        stdout: 'A'.repeat(500),
        stderr: '',
        compilation_output: '',
        stdout_truncated: true,
        stderr_truncated: false,
        exit_code: 0,
        execution_time_ms: 80,
        memory_usage_kb: 10240,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      error: null,
      isRunning: false,
    };

    render(<ResultConsole state={truncatedState} />);

    expect(screen.getByText(/output truncated at 64 kb limit\./i)).toBeInTheDocument();
  });

  it('8. displays TIME_LIMIT_EXCEEDED explanation alert', () => {
    const tleState: ExecutionState = {
      status: 'terminal',
      submissionId: 'sub-tle-1',
      submission: {
        id: 'sub-tle-1',
        status: 'TIME_LIMIT_EXCEEDED',
        stdout: '',
        stderr: '',
        compilation_output: '',
        stdout_truncated: false,
        stderr_truncated: false,
        exit_code: 137,
        execution_time_ms: 2050,
        memory_usage_kb: 12000,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      error: null,
      isRunning: false,
    };

    render(<ResultConsole state={tleState} />);

    expect(screen.getByText('TIME_LIMIT_EXCEEDED')).toBeInTheDocument();
    expect(screen.getByText(/the program exceeded the execution time limit/i)).toBeInTheDocument();
  });

  it('9. displays MEMORY_LIMIT_EXCEEDED explanation alert', () => {
    const mleState: ExecutionState = {
      status: 'terminal',
      submissionId: 'sub-mle-1',
      submission: {
        id: 'sub-mle-1',
        status: 'MEMORY_LIMIT_EXCEEDED',
        stdout: '',
        stderr: '',
        compilation_output: '',
        stdout_truncated: false,
        stderr_truncated: false,
        exit_code: 137,
        execution_time_ms: 500,
        memory_usage_kb: 131072, // 128 MB
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      error: null,
      isRunning: false,
    };

    render(<ResultConsole state={mleState} />);

    expect(screen.getByText('MEMORY_LIMIT_EXCEEDED')).toBeInTheDocument();
    expect(screen.getByText(/terminated by the cgroup oom killer/i)).toBeInTheDocument();
  });
});
