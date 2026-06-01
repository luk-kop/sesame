package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"sesame/internal/app"
	"sesame/internal/domain"
)

func TestWriteJSONUsesObjectRoot(t *testing.T) {
	result := domain.ListResult{
		Auth: domain.AuthContext{
			Mode:    domain.AuthModeProfileActive,
			Profile: "dev",
			Region:  "eu-central-1",
		},
		Region:    "eu-central-1",
		Account:   "123456789012",
		ARN:       "arn",
		Warnings:  []domain.Warning{{Code: "partial", Message: "warning"}},
		Instances: []domain.Instance{{ID: "i-1", Name: "api", State: "running", Region: "eu-central-1", SSMStatus: domain.SSMStatusOnline}},
	}

	var buf bytes.Buffer
	if err := writeJSON(&buf, result); err != nil {
		t.Fatalf("writeJSON returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := decoded["instances"]; !ok {
		t.Fatalf("expected object root with instances field, got %s", buf.String())
	}
	if _, ok := decoded["warnings"]; !ok {
		t.Fatalf("expected warnings field, got %s", buf.String())
	}
}

func TestWriteJSONKeepsEmptyArrays(t *testing.T) {
	result := domain.ListResult{
		Auth:      domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		Region:    "eu-central-1",
		Account:   "123456789012",
		ARN:       "arn",
		Warnings:  []domain.Warning{},
		Instances: []domain.Instance{},
	}

	var buf bytes.Buffer
	if err := writeJSON(&buf, result); err != nil {
		t.Fatalf("writeJSON returned error: %v", err)
	}

	var decoded struct {
		Warnings  []domain.Warning  `json:"warnings"`
		Instances []domain.Instance `json:"instances"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded.Warnings == nil {
		t.Fatalf("expected warnings to be an empty array, got null/missing: %s", buf.String())
	}
	if decoded.Instances == nil {
		t.Fatalf("expected instances to be an empty array, got null/missing: %s", buf.String())
	}
}

func TestWriteTableRendersHumanColumns(t *testing.T) {
	result := domain.ListResult{
		Auth:    domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		Region:  "eu-central-1",
		Account: "123456789012",
		ARN:     "arn",
		Instances: []domain.Instance{{
			ID:        "i-1",
			Name:      "api",
			State:     "running",
			SSMStatus: domain.SSMStatusOnline,
			PrivateIP: "10.0.0.1",
			Region:    "eu-central-1",
		}},
	}

	var buf bytes.Buffer
	if err := writeTable(&buf, result); err != nil {
		t.Fatalf("writeTable returned error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"AUTH", "profile-active dev", "ACCOUNT", "123456789012", "NAME", "INSTANCE ID", "api", "i-1", "online", "10.0.0.1", "eu-central-1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestListCommandInvalidFilterReturnsUsageBeforeAWSConfig(t *testing.T) {
	cmd := newRootCommand(&globalOptions{}, BuildInfo{})
	cmd.SetArgs([]string{"list", "--ssm", "ready"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *app.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T %[1]v", err)
	}
	if exitErr.Code != app.ExitUsageError {
		t.Fatalf("expected usage exit code, got %d", exitErr.Code)
	}
	if !strings.Contains(exitErr.Error(), "unsupported SSM status") {
		t.Fatalf("expected SSM status validation error, got %q", exitErr.Error())
	}
}

func TestTunnelCommandInvalidPortReturnsUsageBeforeDependencies(t *testing.T) {
	cmd := newRootCommand(&globalOptions{}, BuildInfo{})
	cmd.SetArgs([]string{"tunnel", "i-123", "--local-port", "70000", "--remote-port", "5432"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *app.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T %[1]v", err)
	}
	if exitErr.Code != app.ExitUsageError {
		t.Fatalf("expected usage exit code, got %d", exitErr.Code)
	}
	if !strings.Contains(exitErr.Error(), "local-port must be in range") {
		t.Fatalf("expected local port validation error, got %q", exitErr.Error())
	}
}
