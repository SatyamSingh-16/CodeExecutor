import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act, waitFor } from '@testing-library/react';
import React, { useState } from 'react';
import { AuthProvider } from './AuthProvider';
import { useAuth } from './useAuth';
import { authStorage } from './storage';
import { api } from '../api/client';

const TestConsumer: React.FC = () => {
  const { status, user, token, isAuthenticated, login, register, logout } = useAuth();
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const handleLogin = async () => {
    try {
      await login({ email: 'test@example.com', password: 'password123' });
    } catch (e: any) {
      setErrorMsg(e.message);
    }
  };

  const handleRegister = async () => {
    try {
      await register({ email: 'new@example.com', password: 'password123' });
    } catch (e: any) {
      setErrorMsg(e.message);
    }
  };

  return (
    <div>
      <div data-testid="status">{status}</div>
      <div data-testid="authenticated">{isAuthenticated ? 'yes' : 'no'}</div>
      <div data-testid="user-email">{user?.email || 'none'}</div>
      <div data-testid="token">{token || 'none'}</div>
      <div data-testid="error-msg">{errorMsg || 'none'}</div>
      <button onClick={handleLogin}>Trigger Login</button>
      <button onClick={handleRegister}>Trigger Register</button>
      <button onClick={logout}>Trigger Logout</button>
    </div>
  );
};

describe('AuthProvider', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('1. initializes in unauthenticated state when no stored token exists', async () => {
    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('status')).toHaveTextContent('unauthenticated');
    });
    expect(screen.getByTestId('authenticated')).toHaveTextContent('no');
    expect(screen.getByTestId('user-email')).toHaveTextContent('none');
    expect(screen.getByTestId('token')).toHaveTextContent('none');
  });

  it('2. restores authenticated state on mount when valid stored token exists', async () => {
    authStorage.setToken('stored-valid-jwt');
    authStorage.setUser({
      id: 'u-1',
      email: 'persisted@example.com',
      created_at: new Date().toISOString(),
    });

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        user: { id: 'u-1', email: 'persisted@example.com', created_at: new Date().toISOString() },
      }),
    });

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('status')).toHaveTextContent('authenticated');
    });
    expect(screen.getByTestId('authenticated')).toHaveTextContent('yes');
    expect(screen.getByTestId('user-email')).toHaveTextContent('persisted@example.com');
    expect(screen.getByTestId('token')).toHaveTextContent('stored-valid-jwt');
  });

  it('3 & 12. login success updates state, storage, and configures ApiClient Bearer token', async () => {
    const mockAuthResponse = {
      user: { id: 'u-2', email: 'test@example.com', created_at: new Date().toISOString() },
      token: 'newly-issued-jwt-token',
    };

    global.fetch = vi.fn().mockImplementation((url: string) => {
      if (url.endsWith('/api/auth/login')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: new Headers({ 'content-type': 'application/json' }),
          json: async () => mockAuthResponse,
        });
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ status: 'ok' }),
      });
    });

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('status')).toHaveTextContent('unauthenticated');
    });

    await act(async () => {
      screen.getByText('Trigger Login').click();
    });

    expect(screen.getByTestId('status')).toHaveTextContent('authenticated');
    expect(screen.getByTestId('user-email')).toHaveTextContent('test@example.com');
    expect(screen.getByTestId('token')).toHaveTextContent('newly-issued-jwt-token');

    // Verify localStorage persistence
    expect(authStorage.getToken()).toBe('newly-issued-jwt-token');
    expect(authStorage.getUser()?.email).toBe('test@example.com');

    // Verify ApiClient receives Bearer token
    await api.get('/api/test-protected');
    const calls = (global.fetch as any).mock.calls;
    const protectedCall = calls.find((call: any[]) => call[0].endsWith('/api/test-protected'));
    expect(protectedCall).toBeDefined();
    expect(protectedCall[1].headers.get('Authorization')).toBe('Bearer newly-issued-jwt-token');
  });

  it('4. login failure leaves state unauthenticated and throws ApiError', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ error: 'invalid email or password' }),
    });

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('status')).toHaveTextContent('unauthenticated');
    });

    await act(async () => {
      screen.getByText('Trigger Login').click();
    });

    expect(screen.getByTestId('error-msg')).toHaveTextContent('invalid email or password');
    expect(screen.getByTestId('status')).toHaveTextContent('unauthenticated');
    expect(authStorage.getToken()).toBeNull();
  });

  it('5. registration success establishes session and stores token', async () => {
    const mockAuthResponse = {
      user: { id: 'u-3', email: 'new@example.com', created_at: new Date().toISOString() },
      token: 'registered-jwt-token',
    };

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => mockAuthResponse,
    });

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('status')).toHaveTextContent('unauthenticated');
    });

    await act(async () => {
      screen.getByText('Trigger Register').click();
    });

    expect(screen.getByTestId('status')).toHaveTextContent('authenticated');
    expect(screen.getByTestId('user-email')).toHaveTextContent('new@example.com');
    expect(screen.getByTestId('token')).toHaveTextContent('registered-jwt-token');
    expect(authStorage.getToken()).toBe('registered-jwt-token');
  });

  it('8. logout clears session state, storage, and subsequent Authorization headers', async () => {
    authStorage.setToken('token-to-be-cleared');
    authStorage.setUser({
      id: 'u-1',
      email: 'logged-in@example.com',
      created_at: new Date().toISOString(),
    });

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        user: { id: 'u-1', email: 'logged-in@example.com', created_at: new Date().toISOString() },
      }),
    });

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('status')).toHaveTextContent('authenticated');
    });

    act(() => {
      screen.getByText('Trigger Logout').click();
    });

    expect(screen.getByTestId('status')).toHaveTextContent('unauthenticated');
    expect(screen.getByTestId('authenticated')).toHaveTextContent('no');
    expect(screen.getByTestId('user-email')).toHaveTextContent('none');
    expect(authStorage.getToken()).toBeNull();
    expect(authStorage.getUser()).toBeNull();

    // Verify subsequent request does NOT have Authorization header
    await api.get('/api/health');
    const lastCall = (global.fetch as any).mock.calls[(global.fetch as any).mock.calls.length - 1];
    expect(lastCall[1].headers.get('Authorization')).toBeNull();
  });

  it('14. 401 response on protected endpoint automatically invalidates session', async () => {
    authStorage.setToken('expired-jwt-token');
    authStorage.setUser({
      id: 'u-1',
      email: 'expired@example.com',
      created_at: new Date().toISOString(),
    });

    // Verification on mount succeeds initially
    global.fetch = vi.fn().mockImplementation((url: string) => {
      if (url.endsWith('/api/auth/me')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: new Headers({ 'content-type': 'application/json' }),
          json: async () => ({
            user: { id: 'u-1', email: 'expired@example.com', created_at: new Date().toISOString() },
          }),
        });
      }
      // Protected API endpoint returns 401 Unauthorized
      return Promise.resolve({
        ok: false,
        status: 401,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ error: 'invalid or expired token' }),
      });
    });

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('status')).toHaveTextContent('authenticated');
    });

    // Fire protected API call that receives 401
    await act(async () => {
      try {
        await api.get('/api/submissions');
      } catch {
        // Expected ApiError
      }
    });

    // State should automatically transition to unauthenticated and clear storage
    expect(screen.getByTestId('status')).toHaveTextContent('unauthenticated');
    expect(authStorage.getToken()).toBeNull();
    expect(authStorage.getUser()).toBeNull();
  });
});
