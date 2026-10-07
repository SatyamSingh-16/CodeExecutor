import type { SupportedLanguage } from '../types/submission';

export const STARTER_TEMPLATES: Record<SupportedLanguage, string> = {
  python: `import sys

def main():
    # Read standard input
    data = sys.stdin.read()
    if data:
        print(f"Received input:\\n{data.strip()}")
    else:
        print("Hello, Distributed Code Execution Platform!")

if __name__ == "__main__":
    main()
`,
  go: `package main

import (
\t"fmt"
\t"io"
\t"os"
\t"strings"
)

func main() {
\t// Read standard input
\tdata, err := io.ReadAll(os.Stdin)
\tif err == nil && len(data) > 0 {
\t\tfmt.Printf("Received input:\\n%s\\n", strings.TrimSpace(string(data)))
\t} else {
\t\tfmt.Println("Hello, Distributed Code Execution Platform!")
\t}
}
`,
};

export const DEFAULT_LANGUAGE: SupportedLanguage = 'python';

export function getStarterTemplate(lang: SupportedLanguage): string {
  return STARTER_TEMPLATES[lang] || '';
}
