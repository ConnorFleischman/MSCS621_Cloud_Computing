package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRejectsInvalidDays(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "-topic", "cloud computing", "-days", "0", "-articles", "1")
	cmd.Dir = filepath.Join("..")
	cmd.Env = append(os.Environ(), "NEWSAPI_API_KEY=dummy")

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected invalid days to fail, but command succeeded: %s", output)
	}
	if !strings.Contains(string(output), "days must be at least 1") {
		t.Fatalf("expected validation error, got: %s", output)
	}
}
