package converter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConcatFile(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "concat.txt")

	files := []string{"seg_000000.ts", "seg_000001.ts"}
	if err := writeConcatFile(listPath, files); err != nil {
		t.Fatalf("writeConcatFile: %v", err)
	}

	data, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("read concat file: %v", err)
	}

	var wantLines []string
	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			t.Fatalf("filepath.Abs: %v", err)
		}
		wantLines = append(wantLines, fmt.Sprintf("file '%s'", abs))
	}
	want := strings.Join(wantLines, "\n") + "\n"

	if string(data) != want {
		t.Errorf("concat file = %q, want %q", data, want)
	}
}

func TestWriteConcatFileEscapesQuotes(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "concat.txt")

	if err := writeConcatFile(listPath, []string{"it's.ts"}); err != nil {
		t.Fatalf("writeConcatFile: %v", err)
	}

	data, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("read concat file: %v", err)
	}

	abs, err := filepath.Abs("it's.ts")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	want := fmt.Sprintf("file '%s'\n", strings.ReplaceAll(abs, "'", "'\\''"))

	if string(data) != want {
		t.Errorf("concat file = %q, want %q", data, want)
	}
}
