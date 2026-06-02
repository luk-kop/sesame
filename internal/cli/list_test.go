package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

	for _, want := range []string{"AUTH", "profile-active dev", "ACCOUNT", "123456789012", "INSTANCES", "1", "#", "NAME", "INSTANCE ID", "api", "i-1", "online", "10.0.0.1", "eu-central-1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestWriteTableLimitShowsOnlyRequestedRows(t *testing.T) {
	result := domain.ListResult{
		Auth:      domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		Region:    "eu-central-1",
		Account:   "123456789012",
		ARN:       "arn",
		Instances: testListInstances(3),
	}

	var buf bytes.Buffer
	if err := writeTableWithOptions(&buf, result, tableOptions{Limit: 2}); err != nil {
		t.Fatalf("writeTableWithOptions returned error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"INSTANCES  3", "SHOWN      2", "api-1", "api-2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected limited table output to contain %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "api-3") {
		t.Fatalf("expected limited table output to hide third row, got:\n%s", out)
	}
}

func TestWriteTableLongARNDoesNotWidenInstanceColumns(t *testing.T) {
	result := domain.ListResult{
		Auth:    domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		Region:  "eu-central-1",
		Account: "123456789012",
		ARN:     strings.Repeat("arn:", 40),
		Instances: []domain.Instance{{
			ID:        "i-1",
			Name:      "api",
			State:     "running",
			SSMStatus: domain.SSMStatusOnline,
			Region:    "eu-central-1",
		}},
	}

	var buf bytes.Buffer
	if err := writeTable(&buf, result); err != nil {
		t.Fatalf("writeTable returned error: %v", err)
	}

	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "api") && strings.Contains(line, "i-1") && strings.Contains(line, strings.Repeat(" ", 30)) {
			t.Fatalf("expected long ARN metadata not to widen instance row, got line %q\n%s", line, buf.String())
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

func TestListCommandInvalidLimitReturnsUsageBeforeAWSConfig(t *testing.T) {
	cmd := newRootCommand(&globalOptions{}, BuildInfo{})
	cmd.SetArgs([]string{"list", "--limit", "-1"})
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
	if !strings.Contains(exitErr.Error(), "limit must be greater than or equal to 0") {
		t.Fatalf("expected limit validation error, got %q", exitErr.Error())
	}
}

func testListInstances(count int) []domain.Instance {
	instances := make([]domain.Instance, count)
	for i := range instances {
		instances[i] = domain.Instance{
			ID:        fmt.Sprintf("i-%d", i+1),
			Name:      fmt.Sprintf("api-%d", i+1),
			State:     "running",
			SSMStatus: domain.SSMStatusOnline,
			Region:    "eu-central-1",
		}
	}
	return instances
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
