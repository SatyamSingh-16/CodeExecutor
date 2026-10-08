import { describe, it, expect, vi, beforeEach } from 'vitest';
import { listSubmissions, getSubmission } from './api';
import { api } from '../api/client';

vi.mock('../api/client', () => ({
  api: {
    get: vi.fn(),
  },
}));

describe('history api', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('listSubmissions', () => {
    it('calls GET /api/submissions with default page=1 and limit=20', async () => {
      const mockData = [{ id: 'sub-1', language: 'python', status: 'SUCCESS' }];
      (api.get as any).mockResolvedValueOnce(mockData);

      const result = await listSubmissions();

      expect(api.get).toHaveBeenCalledWith('/api/submissions?page=1&limit=20');
      expect(result).toEqual(mockData);
    });

    it('passes custom page and limit parameters without client-controlled user_id', async () => {
      (api.get as any).mockResolvedValueOnce([]);

      await listSubmissions(3, 15);

      expect(api.get).toHaveBeenCalledWith('/api/submissions?page=3&limit=15');
      const calledUrl = (api.get as any).mock.calls[0][0];
      expect(calledUrl).not.toContain('user_id');
    });

    it('propagates API errors upwards', async () => {
      (api.get as any).mockRejectedValueOnce(new Error('Network error'));

      await expect(listSubmissions()).rejects.toThrow('Network error');
    });
  });

  describe('getSubmission', () => {
    it('calls GET /api/submissions/:id', async () => {
      const mockDetail = { id: 'sub-xyz', language: 'go', status: 'SUCCESS' };
      (api.get as any).mockResolvedValueOnce(mockDetail);

      const result = await getSubmission('sub-xyz');

      expect(api.get).toHaveBeenCalledWith('/api/submissions/sub-xyz');
      expect(result).toEqual(mockDetail);
    });
  });
});
