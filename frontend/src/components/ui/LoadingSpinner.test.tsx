import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { LoadingSpinner } from './LoadingSpinner';

describe('LoadingSpinner component', () => {
  it('renders with role="status" and default accessible label', () => {
    render(<LoadingSpinner />);
    const spinner = screen.getByRole('status');
    expect(spinner).toBeInTheDocument();
    expect(screen.getByText('Loading...')).toBeInTheDocument();
  });

  it('renders custom accessible label', () => {
    render(<LoadingSpinner label="Executing code in sandbox..." />);
    expect(screen.getByText('Executing code in sandbox...')).toBeInTheDocument();
  });
});
