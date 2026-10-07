import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { Header } from './Header';
import { AuthContext } from '../../auth/AuthContext';
import type { AuthContextValue } from '../../auth/types';

describe('Header authentication states', () => {
  const mockLogout = vi.fn();

  beforeEach(() => {
    mockLogout.mockReset();
  });

  const renderHeader = (authOverrides: Partial<AuthContextValue>) => {
    const authValue: AuthContextValue = {
      status: 'unauthenticated',
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      login: vi.fn(),
      register: vi.fn(),
      logout: mockLogout,
      ...authOverrides,
    };

    return render(
      <MemoryRouter>
        <AuthContext.Provider value={authValue}>
          <Header />
        </AuthContext.Provider>
      </MemoryRouter>
    );
  };

  it('renders Login and Register navigation links when unauthenticated', () => {
    renderHeader({ isAuthenticated: false, user: null });

    const nav = screen.getByRole('navigation', { name: /main navigation/i });
    expect(within(nav).getByRole('link', { name: 'Workspace' })).toBeInTheDocument();
    expect(within(nav).getByRole('link', { name: 'Login' })).toBeInTheDocument();
    expect(within(nav).getByRole('link', { name: 'Register' })).toBeInTheDocument();

    expect(screen.queryByRole('button', { name: /sign out/i })).not.toBeInTheDocument();
  });

  it('15. renders user identity and Sign Out button when authenticated', () => {
    renderHeader({
      isAuthenticated: true,
      status: 'authenticated',
      user: {
        id: 'u-1',
        email: 'engineer@codeexecutor.io',
        created_at: new Date().toISOString(),
      },
    });

    const nav = screen.getByRole('navigation', { name: /main navigation/i });
    expect(within(nav).getByRole('link', { name: 'Workspace' })).toBeInTheDocument();
    expect(within(nav).queryByRole('link', { name: 'Login' })).not.toBeInTheDocument();
    expect(within(nav).queryByRole('link', { name: 'Register' })).not.toBeInTheDocument();

    expect(screen.getByText('engineer@codeexecutor.io')).toBeInTheDocument();
    const signOutBtn = screen.getByRole('button', { name: /sign out/i });
    expect(signOutBtn).toBeInTheDocument();

    fireEvent.click(signOutBtn);
    expect(mockLogout).toHaveBeenCalledTimes(1);
  });
});
