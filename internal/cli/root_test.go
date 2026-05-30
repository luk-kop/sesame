package cli

import (
	"os"
	"testing"

	"sesame/internal/app"
)

func TestExecuteMapsUsageErrorToExitCode(t *testing.T) {
	withArgs(t, "sesame", "list", "--ssm", "ready")

	if got := Execute(); got != app.ExitUsageError {
		t.Fatalf("expected usage exit code %d, got %d", app.ExitUsageError, got)
	}
}

func TestExecuteMapsMissingDependencyToExitCode(t *testing.T) {
	withArgs(t, "sesame", "shell", "i-123")
	t.Setenv("PATH", "/nonexistent")

	if got := Execute(); got != app.ExitMissingDependency {
		t.Fatalf("expected missing dependency exit code %d, got %d", app.ExitMissingDependency, got)
	}
}

func withArgs(t *testing.T, args ...string) {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string(nil), args...)
	t.Cleanup(func() {
		os.Args = oldArgs
	})
}
