import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Workspace } from './Workspace';
import { STARTER_TEMPLATES } from './templates';

// Clean isolated mock of @monaco-editor/react for jsdom testing
vi.mock('@monaco-editor/react', () => {
  return {
    default: ({ language, value, onChange }: any) => {
      return (
        <div data-testid="mock-monaco-editor" data-language={language}>
          <textarea
            data-testid="mock-monaco-textarea"
            aria-label="Monaco Source Code Editor"
            value={value}
            onChange={(e) => onChange?.(e.target.value)}
          />
        </div>
      );
    },
  };
});

describe('Workspace component', () => {
  it('1. renders workspace container and all panels', () => {
    render(<Workspace />);

    expect(screen.getByRole('button', { name: /run code/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/select programming language/i)).toBeInTheDocument();
    expect(screen.getByTestId('mock-monaco-editor')).toBeInTheDocument();
    expect(screen.getByLabelText(/standard input \(stdin\)/i)).toBeInTheDocument();
    expect(screen.getByText(/execution output/i)).toBeInTheDocument();
  });

  it('2 & 3. defaults to Python language and displays Python starter template', () => {
    render(<Workspace />);

    const languageSelect = screen.getByLabelText(/select programming language/i) as HTMLSelectElement;
    expect(languageSelect.value).toBe('python');

    const editorContainer = screen.getByTestId('mock-monaco-editor');
    expect(editorContainer).toHaveAttribute('data-language', 'python');

    const textarea = screen.getByTestId('mock-monaco-textarea');
    expect(textarea).toHaveValue(STARTER_TEMPLATES.python);
  });

  it('4 & 5. switching language to Go updates mode and loads Go starter template', () => {
    render(<Workspace />);

    const languageSelect = screen.getByLabelText(/select programming language/i);
    fireEvent.change(languageSelect, { target: { value: 'go' } });

    expect(screen.getByTestId('mock-monaco-editor')).toHaveAttribute('data-language', 'go');

    const textarea = screen.getByTestId('mock-monaco-textarea');
    expect(textarea).toHaveValue(STARTER_TEMPLATES.go);
  });

  it('6. allows editing source code', () => {
    render(<Workspace />);

    const textarea = screen.getByTestId('mock-monaco-textarea');
    fireEvent.change(textarea, { target: { value: 'print("Custom Test")' } });

    expect(textarea).toHaveValue('print("Custom Test")');
  });

  it('7. allows editing standard input (stdin)', () => {
    render(<Workspace />);

    const stdinInput = screen.getByLabelText(/standard input \(stdin\)/i);
    fireEvent.change(stdinInput, { target: { value: 'custom input data' } });

    expect(stdinInput).toHaveValue('custom input data');
  });

  it('8. Run Code callback receives language, sourceCode, and stdin', () => {
    const handleRun = vi.fn();
    render(<Workspace onRun={handleRun} />);

    // Modify code and stdin
    fireEvent.change(screen.getByTestId('mock-monaco-textarea'), {
      target: { value: 'print("Running user script")' },
    });
    fireEvent.change(screen.getByLabelText(/standard input \(stdin\)/i), {
      target: { value: '42' },
    });

    const runBtn = screen.getByRole('button', { name: /run code/i });
    fireEvent.click(runBtn);

    expect(handleRun).toHaveBeenCalledTimes(1);
    expect(handleRun).toHaveBeenCalledWith({
      language: 'python',
      sourceCode: 'print("Running user script")',
      stdin: '42',
    });
  });

  it('9. validates empty source code and blocks run callback', () => {
    const handleRun = vi.fn();
    render(<Workspace onRun={handleRun} />);

    // Clear source code
    fireEvent.change(screen.getByTestId('mock-monaco-textarea'), {
      target: { value: '   ' },
    });

    const runBtn = screen.getByRole('button', { name: /run code/i });
    fireEvent.click(runBtn);

    expect(handleRun).not.toHaveBeenCalled();
    expect(screen.getByText(/cannot execute empty source code/i)).toBeInTheDocument();
  });

  it('10, 11 & 12. accessibility controls on language selector, stdin, and run button', () => {
    render(<Workspace />);

    const select = screen.getByLabelText(/select programming language/i);
    expect(select).toBeInTheDocument();
    expect(select).not.toBeDisabled();

    const stdin = screen.getByLabelText(/standard input \(stdin\)/i);
    expect(stdin).toBeInTheDocument();
    expect(stdin).toHaveAttribute('id', 'stdin-input');

    const runBtn = screen.getByRole('button', { name: /run code/i });
    expect(runBtn).toBeInTheDocument();
    expect(runBtn).toHaveAttribute('aria-label', 'Run Code');
  });

  it('14. Reset to Template button restores boilerplate', () => {
    render(<Workspace />);

    const textarea = screen.getByTestId('mock-monaco-textarea');
    fireEvent.change(textarea, { target: { value: '# modified code' } });
    expect(textarea).toHaveValue('# modified code');

    const resetBtn = screen.getByRole('button', { name: /reset to template/i });
    fireEvent.click(resetBtn);

    expect(textarea).toHaveValue(STARTER_TEMPLATES.python);
  });
});
