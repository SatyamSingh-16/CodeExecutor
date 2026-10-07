import React from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../../auth';
import { Button } from '../ui/Button';

export interface HeaderProps {
  appName?: string;
  userSlot?: React.ReactNode;
}

export const Header: React.FC<HeaderProps> = ({
  appName = 'CodeExecutor',
  userSlot,
}) => {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, isAuthenticated, logout } = useAuth();

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const navLinks = isAuthenticated
    ? [{ label: 'Workspace', path: '/app' }]
    : [
        { label: 'Workspace', path: '/app' },
        { label: 'Login', path: '/login' },
        { label: 'Register', path: '/register' },
      ];

  return (
    <header
      style={{
        backgroundColor: 'var(--bg-surface)',
        borderBottom: '1px solid var(--border-subtle)',
        padding: '0 var(--space-6)',
        height: '60px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        position: 'sticky',
        top: 0,
        zIndex: 50,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-6)' }}>
        <Link
          to="/"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--space-2)',
            fontWeight: 'var(--font-bold)',
            fontSize: 'var(--text-base)',
            color: 'var(--text-primary)',
            textDecoration: 'none',
          }}
        >
          <span
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: '28px',
              height: '28px',
              backgroundColor: 'var(--accent-primary)',
              borderRadius: 'var(--radius-sm)',
              color: '#ffffff',
              fontSize: 'var(--text-xs)',
              fontWeight: 'var(--font-bold)',
              fontFamily: 'var(--font-mono)',
            }}
          >
            &gt;_
          </span>
          {appName}
        </Link>

        <nav aria-label="Main Navigation">
          <ul
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--space-4)',
              listStyle: 'none',
            }}
          >
            {navLinks.map((link) => {
              const isActive = location.pathname === link.path;
              return (
                <li key={link.path}>
                  <Link
                    to={link.path}
                    style={{
                      fontSize: 'var(--text-sm)',
                      fontWeight: isActive ? 'var(--font-semibold)' : 'var(--font-medium)',
                      color: isActive ? 'var(--text-primary)' : 'var(--text-secondary)',
                      textDecoration: 'none',
                      padding: 'var(--space-1) var(--space-2)',
                      borderRadius: 'var(--radius-sm)',
                      transition: 'color var(--transition-fast)',
                    }}
                  >
                    {link.label}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
        {userSlot ? (
          userSlot
        ) : isAuthenticated && user ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
            <span
              style={{
                fontSize: 'var(--text-xs)',
                color: 'var(--text-secondary)',
                fontFamily: 'var(--font-mono)',
              }}
              title={user.email}
            >
              {user.email}
            </span>
            <Button
              variant="secondary"
              size="sm"
              onClick={handleLogout}
              aria-label="Sign Out"
            >
              Sign Out
            </Button>
          </div>
        ) : (
          <div style={{ fontSize: 'var(--text-xs)', color: 'var(--text-muted)' }}>
            v0.1.0-alpha
          </div>
        )}
      </div>
    </header>
  );
};
