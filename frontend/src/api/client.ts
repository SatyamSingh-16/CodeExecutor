import { ApiError } from './errors';
import type { ApiErrorResponse } from '../types/api';

export interface ApiClientConfig {
  baseUrl?: string;
  getToken?: () => string | null;
  onUnauthorized?: () => void;
}

export class ApiClient {
  private baseUrl: string;
  private getToken?: () => string | null;
  private onUnauthorized?: () => void;

  constructor(config: ApiClientConfig = {}) {
    this.baseUrl = config.baseUrl || import.meta.env.VITE_API_BASE_URL || '';
    this.getToken = config.getToken;
    this.onUnauthorized = config.onUnauthorized;
  }

  public setTokenGetter(getter: () => string | null) {
    this.getToken = getter;
  }

  public setUnauthorizedHandler(handler: () => void) {
    this.onUnauthorized = handler;
  }

  public async request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    const headers = new Headers(options.headers || {});

    if (!headers.has('Content-Type') && options.body && typeof options.body === 'string') {
      headers.set('Content-Type', 'application/json');
    }

    if (this.getToken) {
      const token = this.getToken();
      if (token && !headers.has('Authorization')) {
        headers.set('Authorization', `Bearer ${token}`);
      }
    }

    let response: Response;
    try {
      response = await fetch(url, {
        ...options,
        headers,
      });
    } catch (err) {
      throw new ApiError(0, err instanceof Error ? err.message : 'Network request failed');
    }

    if (!response.ok) {
      if (
        response.status === 401 &&
        !endpoint.startsWith('/api/auth/login') &&
        !endpoint.startsWith('/api/auth/register')
      ) {
        this.onUnauthorized?.();
      }

      let errorMessage = `HTTP Error ${response.status}`;
      let details: Record<string, string> | undefined;

      try {
        const errorJson = (await response.json()) as ApiErrorResponse;
        if (errorJson.error) {
          errorMessage = errorJson.error;
        }
        details = errorJson.details;
      } catch {
        // Fall back to status text if body isn't JSON
        if (response.statusText) {
          errorMessage = response.statusText;
        }
      }

      throw new ApiError(response.status, errorMessage, details);
    }

    // 204 No Content
    if (response.status === 204) {
      return {} as T;
    }

    return (await response.json()) as T;
  }

  public get<T>(endpoint: string, options?: RequestInit): Promise<T> {
    return this.request<T>(endpoint, { ...options, method: 'GET' });
  }

  public post<T>(endpoint: string, data?: unknown, options?: RequestInit): Promise<T> {
    return this.request<T>(endpoint, {
      ...options,
      method: 'POST',
      body: data !== undefined ? JSON.stringify(data) : undefined,
    });
  }
}

export const api = new ApiClient();
