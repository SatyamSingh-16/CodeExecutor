import type { User } from '../types/auth';

/**
 * Storage keys for auth persistence.
 */
const TOKEN_KEY = 'code_executor_token';
const USER_KEY = 'code_executor_user';

/**
 * Security Tradeoff Documentation:
 *
 * Storing JWT tokens in localStorage provides instant session persistence across page
 * reloads without requiring server-managed session cookies or specialized reverse-proxy cookie paths.
 *
 * Tradeoff:
 * - Advantage: Simple, stateless client session management adhering strictly to the existing
 *   JWT token architecture (T11). Works seamlessly across all client API requests.
 * - Vulnerability: Tokens in localStorage are accessible to any JavaScript running in the same origin,
 *   which makes them vulnerable if an XSS (Cross-Site Scripting) vulnerability is introduced.
 * - Mitigation: We isolate token access strictly within this storage module, never expose
 *   tokens to arbitrary components, and ensure all user inputs are sanitized and rendered
 *   safely via React DOM. In future production hardening, HttpOnly Secure SameSite cookies
 *   could be introduced if backend cookie-issuing endpoints are added.
 */

export const authStorage = {
  getToken(): string | null {
    try {
      return localStorage.getItem(TOKEN_KEY);
    } catch {
      return null;
    }
  },

  setToken(token: string): void {
    try {
      localStorage.setItem(TOKEN_KEY, token);
    } catch (e) {
      console.warn('Unable to persist token to localStorage', e);
    }
  },

  removeToken(): void {
    try {
      localStorage.removeItem(TOKEN_KEY);
    } catch {
      // Ignore storage errors on cleanup
    }
  },

  getUser(): User | null {
    try {
      const raw = localStorage.getItem(USER_KEY);
      if (!raw) return null;
      return JSON.parse(raw) as User;
    } catch {
      return null;
    }
  },

  setUser(user: User): void {
    try {
      localStorage.setItem(USER_KEY, JSON.stringify(user));
    } catch (e) {
      console.warn('Unable to persist user to localStorage', e);
    }
  },

  removeUser(): void {
    try {
      localStorage.removeItem(USER_KEY);
    } catch {
      // Ignore storage errors on cleanup
    }
  },

  clearSession(): void {
    try {
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
    } catch {
      // Ignore storage errors on cleanup
    }
  },
};
