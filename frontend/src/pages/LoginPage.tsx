import React, { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { PageContainer } from '../components/ui/PageContainer';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Alert } from '../components/ui/Alert';
import { useAuth } from '../auth';
import { ApiError } from '../api/errors';

export const LoginPage: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { login } = useAuth();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [fieldErrors, setFieldErrors] = useState<{ email?: string; password?: string }>({});
  const [serverError, setServerError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const validate = (): boolean => {
    const errors: { email?: string; password?: string } = {};

    if (!email.trim()) {
      errors.email = 'Email address is required.';
    } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) {
      errors.email = 'Please enter a valid email address.';
    }

    if (!password) {
      errors.password = 'Password is required.';
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setServerError(null);

    if (isLoading) {
      return;
    }

    if (!validate()) {
      return;
    }

    setIsLoading(true);

    try {
      await login({
        email: email.trim().toLowerCase(),
        password,
      });

      const redirectTarget = searchParams.get('redirect') || '/app';
      navigate(redirectTarget, { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        setServerError(err.message);
      } else {
        setServerError('An unexpected network error occurred. Please check your connection.');
      }
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <PageContainer maxWidth="sm">
      <Card style={{ marginTop: 'var(--space-8)' }}>
        <h2 style={{ marginBottom: 'var(--space-2)' }}>Sign In</h2>
        <p style={{ marginBottom: 'var(--space-6)', color: 'var(--text-secondary)' }}>
          Enter your credentials to access your submissions and editor workspace.
        </p>

        {serverError && (
          <div style={{ marginBottom: 'var(--space-4)' }}>
            <Alert variant="error" title="Authentication Error">
              {serverError}
            </Alert>
          </div>
        )}

        <form onSubmit={handleSubmit} noValidate style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          <Input
            id="email"
            label="Email Address"
            type="email"
            value={email}
            onChange={(e) => {
              setEmail(e.target.value);
              if (fieldErrors.email) {
                setFieldErrors((prev) => ({ ...prev, email: undefined }));
              }
            }}
            placeholder="developer@example.com"
            required
            error={fieldErrors.email}
            disabled={isLoading}
            autoComplete="email"
          />

          <Input
            id="password"
            label="Password"
            type="password"
            value={password}
            onChange={(e) => {
              setPassword(e.target.value);
              if (fieldErrors.password) {
                setFieldErrors((prev) => ({ ...prev, password: undefined }));
              }
            }}
            placeholder="••••••••••••"
            required
            error={fieldErrors.password}
            disabled={isLoading}
            autoComplete="current-password"
          />

          <Button
            type="submit"
            variant="primary"
            isLoading={isLoading}
            disabled={isLoading}
            style={{ width: '100%', marginTop: 'var(--space-2)' }}
          >
            {isLoading ? 'Signing In...' : 'Sign In'}
          </Button>
        </form>

        <div style={{ marginTop: 'var(--space-6)', textAlign: 'center', fontSize: 'var(--text-sm)', color: 'var(--text-secondary)' }}>
          Don't have an account?{' '}
          <Link to="/register" style={{ color: 'var(--accent-primary)', fontWeight: 'var(--font-medium)' }}>
            Create one here
          </Link>
        </div>
      </Card>
    </PageContainer>
  );
};
