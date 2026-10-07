import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { routes } from './router';
import { authStorage } from '../auth/storage';

describe('Application Shell and Routing', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('renders application shell and home page at root route "/"', () => {
    const memoryRouter = createMemoryRouter(routes, {
      initialEntries: ['/'],
    });

    render(<RouterProvider router={memoryRouter} />);

    // Application Branding in Header
    expect(screen.getByText('CodeExecutor')).toBeInTheDocument();

    // Navigation links within Main Navigation
    const nav = screen.getByRole('navigation', { name: /main navigation/i });
    expect(within(nav).getByRole('link', { name: 'Workspace' })).toBeInTheDocument();
    expect(within(nav).getByRole('link', { name: 'Login' })).toBeInTheDocument();
    expect(within(nav).getByRole('link', { name: 'Register' })).toBeInTheDocument();

    // Home Page content
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(/secure, sandboxed code execution at scale/i);

    // Footer
    expect(screen.getByText(/sandboxed docker engine/i)).toBeInTheDocument();
  });

  it('resolves the "/login" route', () => {
    const memoryRouter = createMemoryRouter(routes, {
      initialEntries: ['/login'],
    });

    render(<RouterProvider router={memoryRouter} />);

    expect(screen.getByRole('heading', { name: /sign in/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/email address/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('resolves the "/register" route', () => {
    const memoryRouter = createMemoryRouter(routes, {
      initialEntries: ['/register'],
    });

    render(<RouterProvider router={memoryRouter} />);

    expect(screen.getByRole('heading', { name: /create account/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/email address/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/^password/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/confirm password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /register/i })).toBeInTheDocument();
  });

  it('redirects unauthenticated users attempting "/app" to "/login"', () => {
    const memoryRouter = createMemoryRouter(routes, {
      initialEntries: ['/app'],
    });

    render(<RouterProvider router={memoryRouter} />);

    // Unauthenticated user should be redirected to Login page
    expect(screen.getByRole('heading', { name: /sign in/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/email address/i)).toBeInTheDocument();
  });

  it('renders the "/app" workspace when authenticated', async () => {
    // Seed authenticated session in localStorage
    authStorage.setToken('valid-auth-token-123');
    authStorage.setUser({
      id: 'usr-1',
      email: 'dev@example.com',
      created_at: new Date().toISOString(),
    });

    // Mock verification endpoint
    const originalFetch = global.fetch;
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        user: { id: 'usr-1', email: 'dev@example.com', created_at: new Date().toISOString() },
      }),
    });

    try {
      const memoryRouter = createMemoryRouter(routes, {
        initialEntries: ['/app'],
      });

      render(<RouterProvider router={memoryRouter} />);

      expect(await screen.findByRole('heading', { name: /execution workspace/i })).toBeInTheDocument();
      expect(screen.getByText(/monaco editor container will be integrated in ticket 20/i)).toBeInTheDocument();
    } finally {
      global.fetch = originalFetch;
    }
  });

  it('resolves 404 for unknown route paths', () => {
    const memoryRouter = createMemoryRouter(routes, {
      initialEntries: ['/some/non-existent/route'],
    });

    render(<RouterProvider router={memoryRouter} />);

    expect(screen.getByText('404')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /page not found/i })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /return to home/i })).toBeInTheDocument();
  });
});
