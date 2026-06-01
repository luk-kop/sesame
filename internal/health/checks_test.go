package health

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDependencyStatusError(t *testing.T) {
	tests := []struct {
		name    string
		status  DependencyStatus
		wantErr string
	}{
		{
			name: "all dependencies available",
			status: DependencyStatus{
				AWSCLI:               true,
				SessionManagerPlugin: true,
			},
		},
		{
			name: "missing aws cli",
			status: DependencyStatus{
				AWSCLI:               false,
				SessionManagerPlugin: true,
			},
			wantErr: "aws CLI not found in PATH",
		},
		{
			name: "missing session manager plugin",
			status: DependencyStatus{
				AWSCLI:               true,
				SessionManagerPlugin: false,
			},
			wantErr: "session-manager-plugin not found in PATH",
		},
		{
			name: "missing both dependencies reports aws first",
			status: DependencyStatus{
				AWSCLI:               false,
				SessionManagerPlugin: false,
			},
			wantErr: "aws CLI not found in PATH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.status.Error()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestCheckSessionDependencies(t *testing.T) {
	tests := []struct {
		name        string
		executables []string
		want        DependencyStatus
	}{
		{
			name:        "both dependencies available",
			executables: []string{"aws", "session-manager-plugin"},
			want: DependencyStatus{
				AWSCLI:               true,
				SessionManagerPlugin: true,
			},
		},
		{
			name:        "only aws cli available",
			executables: []string{"aws"},
			want: DependencyStatus{
				AWSCLI:               true,
				SessionManagerPlugin: false,
			},
		},
		{
			name:        "only session manager plugin available",
			executables: []string{"session-manager-plugin"},
			want: DependencyStatus{
				AWSCLI:               false,
				SessionManagerPlugin: true,
			},
		},
		{
			name: "no dependencies available",
			want: DependencyStatus{
				AWSCLI:               false,
				SessionManagerPlugin: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			for _, executable := range tt.executables {
				writeExecutable(t, binDir, executable)
			}
			t.Setenv("PATH", binDir)

			got := CheckSessionDependencies()
			if got != tt.want {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
		})
	}
}

func writeExecutable(t *testing.T, dir, name string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		path += ".exe"
	}

	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake executable %s: %v", name, err)
	}
}
