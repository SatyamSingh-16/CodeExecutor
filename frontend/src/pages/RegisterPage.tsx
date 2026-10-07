import React from 'react';
import { Link } from 'react-router-dom';
import { PageContainer } from '../components/ui/PageContainer';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';

export const RegisterPage: React.FC = () => {
  return (
    <PageContainer maxWidth="sm">
      <Card style={{ marginTop: 'var(--space-8)' }}>
        <h2 style={{ marginBottom: 'var(--space-2)' }}>Create Account</h2>
        <p style={{ marginBottom: 'var(--space-6)' }}>
          Register to submit code, stream execution output, and track runtime metrics.
        </p>

        <form onSubmit={(e) => e.preventDefault()} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          <Input
            id="username"
            label="Username"
            placeholder="developer"
            required
            autoComplete="username"
          />

          <Input
            id="email"
            label="Email Address"
            type="email"
            placeholder="developer@example.com"
            required
            autoComplete="email"
          />

          <Input
            id="password"
            label="Password"
            type="password"
            placeholder="••••••••••••"
            required
            helperText="At least 8 characters recommended."
            autoComplete="new-password"
          />

          <Button type="submit" variant="primary" style={{ width: '100%', marginTop: 'var(--space-2)' }}>
            Register
          </Button>
        </form>

        <div style={{ marginTop: 'var(--space-6)', textAlign: 'center', fontSize: 'var(--text-sm)' }}>
          Already have an account?{' '}
          <Link to="/login">Sign in here</Link>
        </div>
      </Card>
    </PageContainer>
  );
};
