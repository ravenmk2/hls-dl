package converter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Convert(ctx context.Context, segmentFiles []string, outputPath string) error {
	listPath := outputPath + ".concat.txt"
	if err := writeConcatFile(listPath, segmentFiles); err != nil {
		return fmt.Errorf("write concat file: %w", err)
	}
	defer os.Remove(listPath)

	absList, err := filepath.Abs(listPath)
	if err != nil {
		return fmt.Errorf("absolute path: %w", err)
	}

	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", absList,
		"-c", "copy",
		outputPath,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}

	return nil
}

func writeConcatFile(path string, files []string) error {
	var lines []string
	for _, f := range files {
		absPath, err := filepath.Abs(f)
		if err != nil {
			return fmt.Errorf("abs path %s: %w", f, err)
		}
		lines = append(lines, fmt.Sprintf("file '%s'", strings.ReplaceAll(absPath, "'", "'\\''")))
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
