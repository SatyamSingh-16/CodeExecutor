import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ApiClient } from './client';
import { ApiError } from './errors';

describe('ApiClient', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('constructs requests with configured base URL and standard headers', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ status: 'ok' }),
    });
    global.fetch = mockFetch;

    const client = new ApiClient({ baseUrl: 'http://localhost:80' });
    const result = await client.post<{ status: string }>('/api/submissions', { code: 'print(1)' });

    expect(mockFetch).toHaveBeenCalledTimes(1);
    const [url, options] = mockFetch.mock.calls[0];
    expect(url).toBe('http://localhost:80/api/submissions');
    expect(options.method).toBe('POST');
    expect(options.headers.get('Content-Type')).toBe('application/json');
    expect(result).toEqual({ status: 'ok' });
  });

  it('attaches authorization bearer token if provider provides one', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ id: '123' }),
    });
    global.fetch = mockFetch;

    const client = new ApiClient({
      baseUrl: '',
      getToken: () => 'test-jwt-token-xyz',
    });

    await client.post('/api/submissions', { language: 'python', source_code: 'print(1)' });

    expect(mockFetch).toHaveBeenCalledTimes(1);
    const [, options] = mockFetch.mock.calls[0];
    expect(options.headers.get('Authorization')).toBe('Bearer test-jwt-token-xyz');
  });

  it('normalizes API error responses into ApiError instances', async () => {
    const errorBody = {
      error: 'Invalid syntax',
      details: { line: '3' },
    };

    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      statusText: 'Bad Request',
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => errorBody,
    });

    const client = new ApiClient();

    try {
      await client.post('/api/submissions', {});
      expect.fail('Expected request to throw ApiError');
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError);
      const apiErr = err as ApiError;
      expect(apiErr.status).toBe(400);
      expect(apiErr.message).toBe('Invalid syntax');
      expect(apiErr.details).toEqual({ line: '3' });
      expect(apiErr.isRateLimited()).toBe(false);
      expect(apiErr.isUnauthorized()).toBe(false);
      expect(apiErr.isNotFound()).toBe(false);
    }
  });

  it('correctly categorizes HTTP status helper methods', () => {
    const notFound = new ApiError(404, 'Not found');
    expect(notFound.isNotFound()).toBe(true);
    expect(notFound.isUnauthorized()).toBe(false);

    const unauthorized = new ApiError(401, 'Unauthorized');
    expect(unauthorized.isUnauthorized()).toBe(true);
    expect(unauthorized.isRateLimited()).toBe(false);

    const rateLimited = new ApiError(429, 'Rate limit exceeded');
    expect(rateLimited.isRateLimited()).toBe(true);
  });

  it('handles non-JSON error bodies gracefully', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
      statusText: 'Bad Gateway',
      headers: new Headers({ 'content-type': 'text/html' }),
      json: async () => { throw new Error('not json'); },
      text: async () => '<html>502 Bad Gateway</html>',
    });

    const client = new ApiClient();

    try {
      await client.get('/api/submissions');
      expect.fail('Expected request to throw ApiError');
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError);
      const apiErr = err as ApiError;
      expect(apiErr.status).toBe(502);
      expect(apiErr.message).toBe('Bad Gateway');
    }
  });
});
