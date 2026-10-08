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

  it('16. renders History button, opens HistoryDrawer, and isolates active execution state', async () => {
    const listSpy = vi.spyOn(apiModule, 'getSubmission').mockResolvedValue({
      id: 'sub-hist-1',
      language: 'go',
      source_code: 'package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("Restored Go") }',
      stdin: 'hello-history',
      status: 'SUCCESS',
      stdout: 'Restored Go\n',
      stderr: '',
      compilation_output: '',
      stdout_truncated: false,
      stderr_truncated: false,
      exit_code: 0,
      execution_time_ms: 50,
      memory_usage_kb: 1024,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    });

    // Mock history API list
    const originalFetch = global.fetch;
    global.fetch = vi.fn().mockImplementation((url: string) => {
      if (typeof url === 'string' && url.includes('/api/submissions?')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve([
              {
                id: 'sub-hist-1',
                user_id: 'user-1',
                language: 'go',
                status: 'SUCCESS',
                created_at: new Date().toISOString(),
              },
            ]),
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({}),
      } as Response);
    });

    try {
      render(<AppPage />);

      // Active execution console starts in idle
      expect(screen.getByText(/ready to execute/i)).toBeInTheDocument();

      // Open history drawer
      const historyBtn = screen.getByRole('button', { name: /submission history/i });
      await act(async () => {
        fireEvent.click(historyBtn);
      });

      // Drawer dialog is open
      expect(screen.getByRole('dialog', { name: /submission history drawer/i })).toBeInTheDocument();

      // Active execution console is STILL in idle (not mutated by opening history)
      expect(screen.getByText(/ready to execute/i)).toBeInTheDocument();
    } finally {
      global.fetch = originalFetch;
      listSpy.mockRestore();
    }
  });
});
