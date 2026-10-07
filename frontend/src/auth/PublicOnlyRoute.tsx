import React from 'react';
import { Navigate, useSearchParams, Outlet } from 'react-router-dom';
import { useAuth } from './useAuth';
import { LoadingSpinner } from '../components/ui/LoadingSpinner';

export interface PublicOnlyRouteProps {
  children?: React.ReactNode;
}

export const PublicOnlyRoute: React.FC<PublicOnlyRouteProps> = ({ children }) => {
  const { status } = useAuth();
  const [searchParams] = useSearchParams();

  if (status === 'initializing') {
    return (
      <div
        role="status"
        aria-live="polite"
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          minHeight: '60vh',
          gap: 'var(--space-4)',
        }}
      >
        <LoadingSpinner size="lg" label="Verifying session..." />
        <span style={{ fontSize: 'var(--text-sm)', color: 'var(--text-muted)' }}>
          Verifying session...
        </span>
      </div>
    );
  }

  if (status === 'authenticated') {
    const redirectTo = searchParams.get('redirect') || '/app';
    return <Navigate to={redirectTo} replace />;
  }

  return children ? <>{children}</> : <Outlet />;
};
