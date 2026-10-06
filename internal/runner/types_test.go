package runner

import (
	"strings"
	"testing"
)

func TestGetRuntimeConfig(t *testing.T) {
	tests := []struct {
		name         string
		lang         Language
		wantImage    string
		wantFileName string
		wantTmpfs    string
		wantErr      bool
	}{
		{
			name:         "Python configuration",
			lang:         LanguagePython,
			wantImage:    "code-executor-runner-python:latest",
			wantFileName: "main.py",
			wantTmpfs:    "rw,noexec,nosuid,size=64m",
			wantErr:      false,
		},
		{
			name:         "Go configuration",
			lang:         LanguageGo,
			wantImage:    "code-executor-runner-go:latest",
			wantFileName: "main.go",
			wantTmpfs:    "rw,exec,nosuid,size=64m",
			wantErr:      false,
		},
		{
			name:    "Unsupported language",
			lang:    Language("rust"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := GetRuntimeConfig(tc.lang)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tc.lang)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg.Image != tc.wantImage {
				t.Errorf("expected Image %q, got %q", tc.wantImage, cfg.Image)
			}

			if cfg.SourceFileName != tc.wantFileName {
				t.Errorf("expected SourceFileName %q, got %q", tc.wantFileName, cfg.SourceFileName)
			}

			if cfg.TmpfsOptions != tc.wantTmpfs {
				t.Errorf("expected TmpfsOptions %q, got %q", tc.wantTmpfs, cfg.TmpfsOptions)
			}
		})
	}
}

func TestTmpfsOptionsDistinction(t *testing.T) {
	pyCfg, err := GetRuntimeConfig(LanguagePython)
	if err != nil {
		t.Fatalf("failed to get python config: %v", err)
	}

	goCfg, err := GetRuntimeConfig(LanguageGo)
	if err != nil {
		t.Fatalf("failed to get go config: %v", err)
	}

	if !strings.Contains(pyCfg.TmpfsOptions, "noexec") {
		t.Errorf("expected Python tmpfs to contain 'noexec', got %q", pyCfg.TmpfsOptions)
	}

	if !strings.Contains(goCfg.TmpfsOptions, "exec") || strings.Contains(goCfg.TmpfsOptions, "noexec") {
		t.Errorf("expected Go tmpfs to contain 'exec' and not 'noexec', got %q", goCfg.TmpfsOptions)
	}
}
