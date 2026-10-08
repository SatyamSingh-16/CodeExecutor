import { useState, useCallback, useRef } from 'react';
import type { SubmissionDetail } from '../types/submission';
import { listSubmissions, getSubmission } from './api';

export const PAGE_SIZE = 20;

export function useSubmissionHistory() {
  const [isOpen, setIsOpen] = useState(false);
  const [submissions, setSubmissions] = useState<SubmissionDetail[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);

  // Selected submission inspection state
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedSubmission, setSelectedSubmission] = useState<SubmissionDetail | null>(null);
  const [isLoadingDetail, setIsLoadingDetail] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  const isFetchingRef = useRef(false);

  const fetchSubmissions = useCallback(async (targetPage = 1, append = false) => {
    if (isFetchingRef.current) return;
    isFetchingRef.current = true;
    setIsLoading(true);
    setError(null);

    try {
      const data = await listSubmissions(targetPage, PAGE_SIZE);
      setSubmissions((prev) => (append ? [...prev, ...data] : data));
      setPage(targetPage);
      setHasMore(data.length === PAGE_SIZE);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load submission history.');
    } finally {
      setIsLoading(false);
      isFetchingRef.current = false;
    }
  }, []);

  const openDrawer = useCallback(() => {
    setIsOpen(true);
    // Fetch initial page if empty or on open
    void fetchSubmissions(1, false);
  }, [fetchSubmissions]);

  const closeDrawer = useCallback(() => {
    setIsOpen(false);
    setSelectedId(null);
    setSelectedSubmission(null);
    setDetailError(null);
  }, []);

  const refresh = useCallback(() => {
    return fetchSubmissions(1, false);
  }, [fetchSubmissions]);

  const loadMore = useCallback(() => {
    if (isLoading || !hasMore) return;
    return fetchSubmissions(page + 1, true);
  }, [fetchSubmissions, page, isLoading, hasMore]);

  const selectSubmission = useCallback(async (id: string) => {
    setSelectedId(id);
    setIsLoadingDetail(true);
    setDetailError(null);

    try {
      const detail = await getSubmission(id);
      setSelectedSubmission(detail);
    } catch (err) {
      setDetailError(err instanceof Error ? err.message : 'Failed to retrieve submission details.');
    } finally {
      setIsLoadingDetail(false);
    }
  }, []);

  const clearSelection = useCallback(() => {
    setSelectedId(null);
    setSelectedSubmission(null);
    setDetailError(null);
  }, []);

  return {
    isOpen,
    openDrawer,
    closeDrawer,
    submissions,
    isLoading,
    error,
    hasMore,
    refresh,
    loadMore,
    selectedId,
    selectedSubmission,
    isLoadingDetail,
    detailError,
    selectSubmission,
    clearSelection,
  };
}
