import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { LoginPage } from './LoginPage';
import { AuthProvider } from '../auth/AuthProvider';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('LoginPage', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    localStorage.clear();
    mockNavigate.mockReset();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  const renderLoginPage = () => {
    return render(
      <MemoryRouter initialEntries={['/login']}>
        <AuthProvider>
          <LoginPage />
        </AuthProvider>
      </MemoryRouter>
    );
  };

  it('renders email input, password input, and sign in button', () => {
    renderLoginPage();

    expect(screen.getByRole('heading', { name: /sign in/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/email address/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('performs client-side required validation and displays errors', async () => {
    renderLoginPage();

    const submitBtn = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitBtn);

    expect(await screen.findByText('Email address is required.')).toBeInTheDocument();
    expect(screen.getByText('Password is required.')).toBeInTheDocument();
  });

  it('validates email syntax client-side', async () => {
    renderLoginPage();

    const emailInput = screen.getByLabelText(/email address/i);
    fireEvent.change(emailInput, { target: { value: 'not-an-email' } });

    const submitBtn = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitBtn);

    expect(await screen.findByText('Please enter a valid email address.')).toBeInTheDocument();
  });

  it('displays backend authentication error message when credentials fail', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ error: 'invalid email or password' }),
    });

    renderLoginPage();

    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'dev@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'wrongpassword' },
    });

    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    expect(await screen.findByText('invalid email or password')).toBeInTheDocument();
  });

  it('navigates to /app upon successful authentication', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        user: { id: 'u-1', email: 'dev@example.com', created_at: new Date().toISOString() },
        token: 'auth-jwt-token-123',
      }),
    });

    renderLoginPage();

    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'dev@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'correctpassword' },
    });

    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/app', { replace: true });
    });
  });

  it('16. prevents duplicate submissions while loading', async () => {
    let resolveLogin: (value: any) => void;
    const loginPromise = new Promise((resolve) => {
      resolveLogin = resolve;
    });

    global.fetch = vi.fn().mockImplementation(() => loginPromise);

    renderLoginPage();

    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'dev@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'correctpassword' },
    });

    const submitBtn = screen.getByRole('button', { name: /sign in/i });
    fireEvent.click(submitBtn);

    // Verify loading state is active and button is disabled
    expect(submitBtn).toBeDisabled();
    expect(submitBtn).toHaveAttribute('aria-busy', 'true');

    // Attempt second click while in flight
    fireEvent.click(submitBtn);

    // Fetch should only have been called once
    expect(global.fetch).toHaveBeenCalledTimes(1);

    // Resolve in-flight request
    resolveLogin!({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        user: { id: 'u-1', email: 'dev@example.com', created_at: new Date().toISOString() },
        token: 'token',
      }),
    });
  });
});
