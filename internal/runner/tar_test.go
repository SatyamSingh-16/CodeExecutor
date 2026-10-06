package runner

import (
	"archive/tar"
	"io"
	"testing"
)

func TestBuildTarArchive(t *testing.T) {
	filename := "main.py"
	content := []byte("print('hello tar')")

	reader, err := BuildTarArchive(filename, content)
	if err != nil {
		t.Fatalf("BuildTarArchive failed: %v", err)
	}

	tr := tar.NewReader(reader)
	header, err := tr.Next()
	if err != nil {
		t.Fatalf("failed to read tar header: %v", err)
	}

	if header.Name != filename {
		t.Errorf("expected header name %q, got %q", filename, header.Name)
	}

	if header.Size != int64(len(content)) {
		t.Errorf("expected header size %d, got %d", len(content), header.Size)
	}

	if header.Mode != 0644 {
		t.Errorf("expected header mode 0644, got %o", header.Mode)
	}

	readBytes, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("failed to read content from tar: %v", err)
	}

	if string(readBytes) != string(content) {
		t.Errorf("expected content %q, got %q", string(content), string(readBytes))
	}

	// Should be EOF now
	_, err = tr.Next()
	if err != io.EOF {
		t.Errorf("expected EOF after single file in tar, got %v", err)
	}
}
