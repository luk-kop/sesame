package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"sesame/internal/app"
)

func TestExecuteMapsUsageErrorToExitCode(t *testing.T) {
	withArgs(t, "sesame", "list", "--ssm", "ready")

	if got := Execute(BuildInfo{}); got != app.ExitUsageError {
		t.Fatalf("expected usage exit code %d, got %d", app.ExitUsageError, got)
	}
}

func TestExecuteMapsMissingDependencyToExitCode(t *testing.T) {
	withArgs(t, "sesame", "shell", "i-123")
	t.Setenv("PATH", "/nonexistent")

	if got := Execute(BuildInfo{}); got != app.ExitMissingDependency {
		t.Fatalf("expected missing dependency exit code %d, got %d", app.ExitMissingDependency, got)
	}
}

func TestRootCommandVersionPrintsBuildInfo(t *testing.T) {
	cmd := newRootCommand(&globalOptions{}, BuildInfo{
		Version:   "v1.2.3",
		Revision:  "abc123",
		BuildDate: "2026-05-31T12:34:56Z",
	})
	cmd.SetArgs([]string{"--version"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected version command to succeed, got %v", err)
	}
	want := "v1.2.3 revision=abc123 build_date=2026-05-31T12:34:56Z\n"
	if stdout.String() != want {
		t.Fatalf("expected version output %q, got %q", want, stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestFormatBuildInfoDefaultsEmptyFields(t *testing.T) {
	got := formatBuildInfo(BuildInfo{})
	for _, want := range []string{"dev", "revision=unknown", "build_date=unknown"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected build info %q to contain %q", got, want)
		}
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
