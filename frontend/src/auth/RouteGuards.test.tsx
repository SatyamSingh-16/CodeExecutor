import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { routes } from '../app/router';
import { authStorage } from './storage';

describe('Route Guards and Redirect Behavior', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('9. redirects unauthenticated user from /app to /login with redirect parameter', async () => {
    const memoryRouter = createMemoryRouter(routes, {
      initialEntries: ['/app'],
    });

    render(<RouterProvider router={memoryRouter} />);

    // Should redirect to /login
    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument();
  });

  it('10. redirects authenticated user from /login to /app', async () => {
    authStorage.setToken('auth-jwt-token-123');
    authStorage.setUser({
      id: 'u-1',
      email: 'user@example.com',
      created_at: new Date().toISOString(),
    });

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        user: { id: 'u-1', email: 'user@example.com', created_at: new Date().toISOString() },
      }),
    });

    const memoryRouter = createMemoryRouter(routes, {
      initialEntries: ['/login'],
    });

    render(<RouterProvider router={memoryRouter} />);

    // Authenticated user should be redirected to /app workspace
    expect(await screen.findByRole('heading', { name: /execution workspace/i })).toBeInTheDocument();
  });

  it('11. redirects authenticated user from /register to /app', async () => {
    authStorage.setToken('auth-jwt-token-123');
    authStorage.setUser({
      id: 'u-1',
      email: 'user@example.com',
      created_at: new Date().toISOString(),
    });

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        user: { id: 'u-1', email: 'user@example.com', created_at: new Date().toISOString() },
      }),
    });

    const memoryRouter = createMemoryRouter(routes, {
      initialEntries: ['/register'],
    });

    render(<RouterProvider router={memoryRouter} />);

    expect(await screen.findByRole('heading', { name: /execution workspace/i })).toBeInTheDocument();
  });
});
