import React from 'react';
import { Link } from 'react-router-dom';
import { PageContainer } from '../components/ui/PageContainer';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';

export const LoginPage: React.FC = () => {
  return (
    <PageContainer maxWidth="sm">
      <Card style={{ marginTop: 'var(--space-8)' }}>
        <h2 style={{ marginBottom: 'var(--space-2)' }}>Sign In</h2>
        <p style={{ marginBottom: 'var(--space-6)' }}>
          Enter your credentials to access your submissions and editor workspace.
        </p>

        <form onSubmit={(e) => e.preventDefault()} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
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
            autoComplete="current-password"
          />

          <Button type="submit" variant="primary" style={{ width: '100%', marginTop: 'var(--space-2)' }}>
            Sign In
          </Button>
        </form>

        <div style={{ marginTop: 'var(--space-6)', textAlign: 'center', fontSize: 'var(--text-sm)' }}>
          Don't have an account?{' '}
          <Link to="/register">Create one here</Link>
        </div>
      </Card>
    </PageContainer>
  );
};
