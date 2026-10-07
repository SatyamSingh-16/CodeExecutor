import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Alert } from './Alert';

describe('Alert component', () => {
  it('renders alert message with title', () => {
    render(
      <Alert title="System Failure" variant="error">
        Container terminated abnormally.
      </Alert>
    );

    const alert = screen.getByRole('alert');
    expect(alert).toBeInTheDocument();
    expect(screen.getByText('System Failure')).toBeInTheDocument();
    expect(screen.getByText('Container terminated abnormally.')).toBeInTheDocument();
  });

  it('renders non-error variants with role="status"', () => {
    render(
      <Alert variant="info">
        Worker node ready.
      </Alert>
    );

    const status = screen.getByRole('status');
    expect(status).toBeInTheDocument();
    expect(screen.getByText('Worker node ready.')).toBeInTheDocument();
  });
});
