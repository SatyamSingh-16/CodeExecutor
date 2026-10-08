import { describe, it, expect } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useWorkspace } from './useWorkspace';
import { STARTER_TEMPLATES } from './templates';

describe('useWorkspace hook', () => {
  it('initializes with Python as default language and its starter template', () => {
    const { result } = renderHook(() => useWorkspace());

    expect(result.current.language).toBe('python');
    expect(result.current.sourceCode).toBe(STARTER_TEMPLATES.python);
    expect(result.current.stdin).toBe('');
  });

  it('updates source code for the active language', () => {
    const { result } = renderHook(() => useWorkspace());

    act(() => {
      result.current.setSourceCode('print("Hello from test")');
    });

    expect(result.current.sourceCode).toBe('print("Hello from test")');
  });

  it('preserves code when switching languages and switches back', () => {
    const { result } = renderHook(() => useWorkspace());

    // Modify Python code
    act(() => {
      result.current.setSourceCode('custom_python_code()');
    });
    expect(result.current.sourceCode).toBe('custom_python_code()');

    // Switch to Go
    act(() => {
      result.current.setLanguage('go');
    });
    expect(result.current.language).toBe('go');
    expect(result.current.sourceCode).toBe(STARTER_TEMPLATES.go);

    // Modify Go code
    act(() => {
      result.current.setSourceCode('custom_go_code()');
    });
    expect(result.current.sourceCode).toBe('custom_go_code()');

    // Switch back to Python and verify custom Python code is preserved
    act(() => {
      result.current.setLanguage('python');
    });
    expect(result.current.language).toBe('python');
    expect(result.current.sourceCode).toBe('custom_python_code()');

    // Switch back to Go and verify custom Go code is preserved
    act(() => {
      result.current.setLanguage('go');
    });
    expect(result.current.language).toBe('go');
    expect(result.current.sourceCode).toBe('custom_go_code()');
  });

  it('updates stdin state', () => {
    const { result } = renderHook(() => useWorkspace());

    act(() => {
      result.current.setStdin('10 20\n30 40');
    });

    expect(result.current.stdin).toBe('10 20\n30 40');
  });

  it('resets code back to starter template when resetToTemplate is called', () => {
    const { result } = renderHook(() => useWorkspace());

    act(() => {
      result.current.setSourceCode('corrupted_code = True');
    });
    expect(result.current.sourceCode).toBe('corrupted_code = True');

    act(() => {
      result.current.resetToTemplate();
    });

    expect(result.current.sourceCode).toBe(STARTER_TEMPLATES.python);
  });

  it('correctly tracks isModified flag when code or stdin changes', () => {
    const { result } = renderHook(() => useWorkspace());

    expect(result.current.isModified).toBe(false);

    act(() => {
      result.current.setStdin('input data');
    });
    expect(result.current.isModified).toBe(true);

    act(() => {
      result.current.setStdin('');
    });
    expect(result.current.isModified).toBe(false);

    act(() => {
      result.current.setSourceCode('print("changed")');
    });
    expect(result.current.isModified).toBe(true);

    act(() => {
      result.current.resetToTemplate();
    });
    expect(result.current.isModified).toBe(false);
  });

  it('loadSubmission restores language, source code, and stdin', () => {
    const { result } = renderHook(() => useWorkspace());

    act(() => {
      result.current.loadSubmission({
        language: 'go',
        sourceCode: 'package main\n\nfunc main() {}',
        stdin: 'sample stdin',
      });
    });

    expect(result.current.language).toBe('go');
    expect(result.current.sourceCode).toBe('package main\n\nfunc main() {}');
    expect(result.current.stdin).toBe('sample stdin');
  });
});
