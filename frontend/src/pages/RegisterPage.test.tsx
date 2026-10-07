import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { RegisterPage } from './RegisterPage';
import { AuthProvider } from '../auth/AuthProvider';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('RegisterPage', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    localStorage.clear();
    mockNavigate.mockReset();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  const renderRegisterPage = () => {
    return render(
      <MemoryRouter initialEntries={['/register']}>
        <AuthProvider>
          <RegisterPage />
        </AuthProvider>
      </MemoryRouter>
    );
  };

  it('renders email, password, and confirm password inputs', () => {
    renderRegisterPage();

    expect(screen.getByRole('heading', { name: /create account/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/email address/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/^password/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/confirm password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /register/i })).toBeInTheDocument();
  });

  it('validates password length >= 8 characters', async () => {
    renderRegisterPage();

    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'user@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/^password/i), {
      target: { value: 'short' },
    });
    fireEvent.change(screen.getByLabelText(/confirm password/i), {
      target: { value: 'short' },
    });

    fireEvent.click(screen.getByRole('button', { name: /register/i }));

    expect(await screen.findByText('Password must be at least 8 characters long.')).toBeInTheDocument();
  });

  it('7. detects and reports password mismatch', async () => {
    renderRegisterPage();

    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'user@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/^password/i), {
      target: { value: 'password123' },
    });
    fireEvent.change(screen.getByLabelText(/confirm password/i), {
      target: { value: 'different123' },
    });

    fireEvent.click(screen.getByRole('button', { name: /register/i }));

    expect(await screen.findByText('Passwords do not match.')).toBeInTheDocument();
  });

  it('displays backend conflict error if email is already registered', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ error: 'email already registered' }),
    });

    renderRegisterPage();

    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'existing@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/^password/i), {
      target: { value: 'password123' },
    });
    fireEvent.change(screen.getByLabelText(/confirm password/i), {
      target: { value: 'password123' },
    });

    fireEvent.click(screen.getByRole('button', { name: /register/i }));

    expect(await screen.findByText('email already registered')).toBeInTheDocument();
  });

  it('5. successfully registers and redirects to /app', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        user: { id: 'u-1', email: 'newdev@example.com', created_at: new Date().toISOString() },
        token: 'new-reg-jwt-token',
      }),
    });

    renderRegisterPage();

    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'newdev@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/^password/i), {
      target: { value: 'securepassword123' },
    });
    fireEvent.change(screen.getByLabelText(/confirm password/i), {
      target: { value: 'securepassword123' },
    });

    fireEvent.click(screen.getByRole('button', { name: /register/i }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/app', { replace: true });
    });
  });

  it('prevents duplicate submissions while loading', async () => {
    let resolveRegister: (value: any) => void;
    const registerPromise = new Promise((resolve) => {
      resolveRegister = resolve;
    });

    global.fetch = vi.fn().mockImplementation(() => registerPromise);

    renderRegisterPage();

    fireEvent.change(screen.getByLabelText(/email address/i), {
      target: { value: 'newdev@example.com' },
    });
    fireEvent.change(screen.getByLabelText(/^password/i), {
      target: { value: 'securepassword123' },
    });
    fireEvent.change(screen.getByLabelText(/confirm password/i), {
      target: { value: 'securepassword123' },
    });

    const submitBtn = screen.getByRole('button', { name: /register/i });
    fireEvent.click(submitBtn);

    expect(submitBtn).toBeDisabled();
    expect(submitBtn).toHaveAttribute('aria-busy', 'true');

    // Attempt second submission
    fireEvent.click(submitBtn);

    expect(global.fetch).toHaveBeenCalledTimes(1);

    resolveRegister!({
      ok: true,
      status: 201,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({
        user: { id: 'u-1', email: 'newdev@example.com', created_at: new Date().toISOString() },
        token: 'token',
      }),
    });
  });
});
