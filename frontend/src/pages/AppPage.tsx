import React from 'react';
import { PageContainer } from '../components/ui/PageContainer';
import { Card } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { Button } from '../components/ui/Button';

export const AppPage: React.FC = () => {
  return (
    <PageContainer maxWidth="xl">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-6)' }}>
        <div>
          <h2>Execution Workspace</h2>
          <p style={{ marginTop: 'var(--space-1)' }}>
            Code editor, live execution sandbox, and real-time streaming output.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 'var(--space-3)', alignItems: 'center' }}>
          <Badge status="QUEUED">Workspace Ready</Badge>
          <Button variant="secondary" size="sm">Settings</Button>
        </div>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))',
          gap: 'var(--space-6)',
        }}
      >
        <Card title="Editor Panel (T20)">
          <div
            style={{
              padding: 'var(--space-12) var(--space-6)',
              textAlign: 'center',
              backgroundColor: 'var(--surface-sunken)',
              border: '1px dashed var(--border-default)',
              borderRadius: 'var(--radius-md)',
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--text-sm)',
              color: 'var(--text-muted)',
            }}
          >
            Monaco Editor container will be integrated in Ticket 20.
          </div>
        </Card>

        <Card title="Execution Output (T21)">
          <div
            style={{
              padding: 'var(--space-12) var(--space-6)',
              textAlign: 'center',
              backgroundColor: 'var(--surface-sunken)',
              border: '1px dashed var(--border-default)',
              borderRadius: 'var(--radius-md)',
              fontFamily: 'var(--font-mono)',
              fontSize: 'var(--text-sm)',
              color: 'var(--text-muted)',
            }}
          >
            Live SSE execution console stream will be integrated in Ticket 21.
          </div>
        </Card>
      </div>
    </PageContainer>
  );
};
