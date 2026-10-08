import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { AppPage } from './AppPage';

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

import * as apiModule from '../execution/api';

describe('AppPage component integration', () => {
  it('15. renders Execution Workspace heading and integrates Workspace component', async () => {
    vi.spyOn(apiModule, 'submitCode').mockResolvedValueOnce({
      id: 'sub-app-1',
      status: 'QUEUED',
    });

    const originalFetch = global.fetch;
    global.fetch = vi.fn().mockReturnValue(
      new Promise(() => {}) // pending stream
    );

    try {
      const handleRun = vi.fn();
      render(<AppPage onRun={handleRun} />);

      expect(screen.getByRole('heading', { name: /execution workspace/i })).toBeInTheDocument();
      expect(screen.getByLabelText(/select programming language/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /run code/i })).toBeInTheDocument();

      // Trigger run code from page
      const runBtn = screen.getByRole('button', { name: /run code/i });
      await act(async () => {
        fireEvent.click(runBtn);
      });

      expect(handleRun).toHaveBeenCalledTimes(1);
      expect(handleRun).toHaveBeenCalledWith(
        expect.objectContaining({
          language: 'python',
          stdin: '',
        })
      );
    } finally {
      global.fetch = originalFetch;
    }
  });
});
