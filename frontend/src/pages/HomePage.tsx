import React from 'react';
import { Link } from 'react-router-dom';
import { PageContainer } from '../components/ui/PageContainer';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';

export const HomePage: React.FC = () => {
  return (
    <PageContainer maxWidth="lg">
      <div style={{ textAlign: 'center', margin: 'var(--space-12) 0 var(--space-8)' }}>
        <div style={{ marginBottom: 'var(--space-4)' }}>
          <Badge variant="processing">High-Throughput Distributed Sandbox</Badge>
        </div>
        <h1 style={{ fontSize: 'var(--text-3xl)', marginBottom: 'var(--space-4)' }}>
          Secure, Sandboxed Code Execution at Scale
        </h1>
        <p style={{ fontSize: 'var(--text-lg)', maxWidth: '640px', margin: '0 auto var(--space-8)' }}>
          Run Python and Go programs in isolated Linux containers with strict cgroup limits,
          unbuffered real-time SSE streaming, and persistent execution metrics.
        </p>
        <div style={{ display: 'flex', gap: 'var(--space-4)', justifyContent: 'center' }}>
          <Link to="/app">
            <Button size="lg" variant="primary">Launch Workspace</Button>
          </Link>
          <Link to="/login">
            <Button size="lg" variant="secondary">Sign In</Button>
          </Link>
        </div>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))',
          gap: 'var(--space-6)',
          marginTop: 'var(--space-12)',
        }}
      >
        <Card variant="default">
          <h3 style={{ marginBottom: 'var(--space-2)' }}>Two-Phase Compilation</h3>
          <p>
            Compiled languages like Go compile in a dedicated build container before extracting
            the binary into a hardened runtime container with zero network access.
          </p>
        </Card>

        <Card variant="default">
          <h3 style={{ marginBottom: 'var(--space-2)' }}>Redis Streams & Worker Pool</h3>
          <p>
            Queue jobs reliably with consumer groups, automatic PEL orphan reclamation,
            bounded concurrency semaphores, and Pub/Sub event delivery.
          </p>
        </Card>

        <Card variant="default">
          <h3 style={{ marginBottom: 'var(--space-2)' }}>Authoritative PostgreSQL</h3>
          <p>
            ACID persistence guarantees state transitions from QUEUED to terminal SUCCESS,
            with fallback history and strict ownership isolation.
          </p>
        </Card>
      </div>
    </PageContainer>
  );
};
