import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { AuthContext } from './AuthContext';
import { authStorage } from './storage';
import { api } from '../api/client';
import { ApiError } from '../api/errors';
import type { User, LoginRequest, RegisterRequest, AuthResponse } from '../types/auth';
import type { AuthStatus, AuthContextValue } from './types';

export interface AuthProviderProps {
  children: React.ReactNode;
}

export const AuthProvider: React.FC<AuthProviderProps> = ({ children }) => {
  const [token, setToken] = useState<string | null>(() => authStorage.getToken());
  const [user, setUser] = useState<User | null>(() => authStorage.getUser());
  const [status, setStatus] = useState<AuthStatus>('initializing');

  const logout = useCallback(() => {
    authStorage.clearSession();
    setToken(null);
    setUser(null);
    setStatus('unauthenticated');
  }, []);

  const handleUnauthorized = useCallback(() => {
    logout();
  }, [logout]);

  // Connect ApiClient to token storage and 401 handler
  useEffect(() => {
    api.setTokenGetter(() => authStorage.getToken());
    api.setUnauthorizedHandler(handleUnauthorized);
  }, [handleUnauthorized]);

  // Session verification on mount
  useEffect(() => {
    let isMounted = true;

    const verifySession = async () => {
      const storedToken = authStorage.getToken();
      if (!storedToken) {
        if (isMounted) {
          setStatus('unauthenticated');
        }
        return;
      }

      try {
        const response = await api.get<{ user: User }>('/api/auth/me');
        if (isMounted) {
          authStorage.setUser(response.user);
          setUser(response.user);
          setToken(storedToken);
          setStatus('authenticated');
        }
      } catch (err) {
        if (!isMounted) return;

        if (err instanceof ApiError && err.status === 401) {
          authStorage.clearSession();
          setToken(null);
          setUser(null);
          setStatus('unauthenticated');
        } else {
          // If offline/network error but stored user exists, preserve session
          const storedUser = authStorage.getUser();
          if (storedUser) {
            setUser(storedUser);
            setToken(storedToken);
            setStatus('authenticated');
          } else {
            setStatus('unauthenticated');
          }
        }
      }
    };

    verifySession();

    return () => {
      isMounted = false;
    };
  }, []);

  const login = useCallback(async (credentials: LoginRequest) => {
    const response = await api.post<AuthResponse>('/api/auth/login', credentials);
    authStorage.setToken(response.token);
    authStorage.setUser(response.user);
    setToken(response.token);
    setUser(response.user);
    setStatus('authenticated');
  }, []);

  const register = useCallback(async (credentials: RegisterRequest) => {
    const response = await api.post<AuthResponse>('/api/auth/register', credentials);
    authStorage.setToken(response.token);
    authStorage.setUser(response.user);
    setToken(response.token);
    setUser(response.user);
    setStatus('authenticated');
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      token,
      isAuthenticated: status === 'authenticated',
      isLoading: status === 'initializing',
      login,
      register,
      logout,
    }),
    [status, user, token, login, register, logout]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};
