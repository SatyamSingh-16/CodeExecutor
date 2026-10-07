import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { EmptyState } from './EmptyState';
import { Button } from './Button';

describe('EmptyState component', () => {
  it('renders title and description', () => {
    render(
      <EmptyState
        title="No Submissions Found"
        description="Submit your first code payload to view real-time execution results."
      />
    );

    expect(screen.getByText('No Submissions Found')).toBeInTheDocument();
    expect(screen.getByText('Submit your first code payload to view real-time execution results.')).toBeInTheDocument();
  });

  it('renders optional action button', () => {
    const handleAction = vi.fn();
    render(
      <EmptyState
        title="No Submissions"
        action={<Button onClick={handleAction}>New Submission</Button>}
      />
    );

    const button = screen.getByRole('button', { name: /new submission/i });
    expect(button).toBeInTheDocument();
    fireEvent.click(button);
    expect(handleAction).toHaveBeenCalledTimes(1);
  });
});
