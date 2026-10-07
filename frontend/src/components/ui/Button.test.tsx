import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Button } from './Button';

describe('Button component', () => {
  it('renders button with children', () => {
    render(<Button>Click Me</Button>);
    const button = screen.getByRole('button', { name: /click me/i });
    expect(button).toBeInTheDocument();
  });

  it('renders primary variant by default', () => {
    render(<Button>Submit</Button>);
    const button = screen.getByRole('button');
    expect(button).toBeInTheDocument();
  });

  it('handles click events when not disabled', () => {
    const handleClick = vi.fn();
    render(<Button onClick={handleClick}>Action</Button>);
    const button = screen.getByRole('button');
    fireEvent.click(button);
    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it('does not trigger click when disabled', () => {
    const handleClick = vi.fn();
    render(<Button disabled onClick={handleClick}>Disabled</Button>);
    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-disabled', 'true');
    fireEvent.click(button);
    expect(handleClick).not.toHaveBeenCalled();
  });

  it('displays loading spinner and sets aria-busy when isLoading is true', () => {
    render(<Button isLoading>Loading Action</Button>);
    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-busy', 'true');
    expect(button).toBeDisabled();
    expect(screen.getByText('Loading Action')).toBeInTheDocument();
  });
});
