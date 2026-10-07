import React from 'react';
import { Outlet } from 'react-router-dom';
import { Header } from './Header';

export interface AppLayoutProps {
  children?: React.ReactNode;
}

export const AppLayout: React.FC<AppLayoutProps> = ({ children }) => {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        minHeight: '100vh',
        backgroundColor: 'var(--bg-canvas)',
      }}
    >
      <Header />
      <main id="main-content" style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
        {children || <Outlet />}
      </main>
      <footer
        style={{
          borderTop: '1px solid var(--border-subtle)',
          padding: 'var(--space-4) var(--space-6)',
          fontSize: 'var(--text-xs)',
          color: 'var(--text-muted)',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          backgroundColor: 'var(--bg-surface-muted)',
        }}
      >
        <div>Distributed Code Execution Platform</div>
        <div>Sandboxed Docker Engine &bull; Redis Streams &bull; PostgreSQL</div>
      </footer>
    </div>
  );
};
