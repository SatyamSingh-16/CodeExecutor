import { describe, it, expect } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { routes } from './router';

describe('Application Shell and Routing', () => {
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
    expect(screen.getByLabelText(/username/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/email address/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /register/i })).toBeInTheDocument();
  });

  it('resolves the "/app" workspace route', () => {
    const memoryRouter = createMemoryRouter(routes, {
      initialEntries: ['/app'],
    });

    render(<RouterProvider router={memoryRouter} />);

    expect(screen.getByRole('heading', { name: /execution workspace/i })).toBeInTheDocument();
    expect(screen.getByText(/monaco editor container will be integrated in ticket 20/i)).toBeInTheDocument();
    expect(screen.getByText(/live sse execution console stream will be integrated in ticket 21/i)).toBeInTheDocument();
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
