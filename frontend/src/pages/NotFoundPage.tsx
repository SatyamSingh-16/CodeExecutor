import React from 'react';
import { Link } from 'react-router-dom';
import { PageContainer } from '../components/ui/PageContainer';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';

export const NotFoundPage: React.FC = () => {
  return (
    <PageContainer maxWidth="sm">
      <Card style={{ marginTop: 'var(--space-12)', textAlign: 'center' }}>
        <h1 style={{ fontSize: 'var(--text-4xl)', marginBottom: 'var(--space-2)' }}>404</h1>
        <h3 style={{ marginBottom: 'var(--space-4)' }}>Page Not Found</h3>
        <p style={{ marginBottom: 'var(--space-6)' }}>
          The requested path does not exist in the Distributed Code Execution Platform.
        </p>
        <Link to="/">
          <Button variant="primary">Return to Home</Button>
        </Link>
      </Card>
    </PageContainer>
  );
};
