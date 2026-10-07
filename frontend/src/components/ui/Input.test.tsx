import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Input } from './Input';

describe('Input component', () => {
  it('renders input with associated label and id', () => {
    render(<Input id="username" label="Username" placeholder="Enter username" />);
    const input = screen.getByLabelText('Username');
    expect(input).toBeInTheDocument();
    expect(input).toHaveAttribute('id', 'username');
    expect(input).toHaveAttribute('placeholder', 'Enter username');
  });

  it('renders required indicator when required prop is passed', () => {
    render(<Input id="email" label="Email" required />);
    expect(screen.getByText('*')).toBeInTheDocument();
  });

  it('renders helper text associated with aria-describedby', () => {
    render(
      <Input
        id="password"
        label="Password"
        helperText="Minimum 8 characters"
      />
    );
    const helper = screen.getByText('Minimum 8 characters');
    expect(helper).toBeInTheDocument();
    const input = screen.getByLabelText('Password');
    expect(input).toHaveAttribute('aria-describedby', 'password-helper');
  });

  it('renders error message with aria-invalid and aria-errormessage', () => {
    render(
      <Input
        id="email"
        label="Email"
        error="Invalid email address"
      />
    );
    const error = screen.getByRole('alert');
    expect(error).toHaveTextContent('Invalid email address');
    const input = screen.getByLabelText('Email');
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAttribute('aria-errormessage', 'email-error');
  });

  it('supports disabled state', () => {
    render(<Input id="test" label="Test" disabled />);
    const input = screen.getByLabelText('Test');
    expect(input).toBeDisabled();
  });
});
