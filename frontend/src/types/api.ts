export interface ApiErrorResponse {
  error: string;
  details?: Record<string, string>;
}

export interface PaginatedResponse<T> {
  data: T[];
  limit: number;
  offset: number;
}
