import { describe, it, expect, vi, beforeEach } from 'vitest';
import { api } from '../api/client';
import { submitCode, getSubmission } from './api';
import type { CreateSubmissionRequest, SubmissionDetail } from '../types/submission';

describe('Execution API', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('1. submission request contains language, source_code, and stdin', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
      id: 'sub-123',
      status: 'QUEUED',
    });

    const request: CreateSubmissionRequest = {
      language: 'python',
      source_code: 'print("hello")',
      stdin: 'input-data',
    };

    const res = await submitCode(request);

    expect(postSpy).toHaveBeenCalledWith('/api/submissions', {
      language: 'python',
      source_code: 'print("hello")',
      stdin: 'input-data',
    });
    expect(res.id).toBe('sub-123');
    expect(res.status).toBe('QUEUED');
  });

  it('2. submission does not contain user_id', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValueOnce({
      id: 'sub-456',
      status: 'QUEUED',
    });

    // Pass arbitrary object including prohibited fields
    const request = {
      language: 'go',
      source_code: 'package main',
      stdin: '',
      user_id: 'injected-user-id',
      status: 'SUCCESS',
    } as unknown as CreateSubmissionRequest;

    await submitCode(request);

    const callArgs = postSpy.mock.calls[0][1] as Record<string, unknown>;
    expect(callArgs).not.toHaveProperty('user_id');
    expect(callArgs).not.toHaveProperty('status');
    expect(callArgs).toEqual({
      language: 'go',
      source_code: 'package main',
      stdin: '',
    });
  });

  it('3. getSubmission fetches authoritative record via GET /api/submissions/:id', async () => {
    const mockDetail: SubmissionDetail = {
      id: 'sub-789',
      language: 'python',
      status: 'SUCCESS',
      stdout: 'done\n',
      stderr: '',
      compilation_output: '',
      stdout_truncated: false,
      stderr_truncated: false,
      exit_code: 0,
      execution_time_ms: 35,
      memory_usage_kb: 8192,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };

    const getSpy = vi.spyOn(api, 'get').mockResolvedValueOnce(mockDetail);

    const detail = await getSubmission('sub-789');

    expect(getSpy).toHaveBeenCalledWith('/api/submissions/sub-789');
    expect(detail).toEqual(mockDetail);
  });
});
