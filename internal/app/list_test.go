package app

import (
	"context"
	"errors"
	"testing"

	"sesame/internal/domain"
)

type fakeInventory struct {
	instances []domain.Instance
	warnings  []domain.Warning
	err       error
}

func (f fakeInventory) ListInstances(context.Context) ([]domain.Instance, []domain.Warning, error) {
	return f.instances, f.warnings, f.err
}

func (f fakeInventory) GetInstance(context.Context, string) (domain.Instance, error) {
	return domain.Instance{}, errors.New("not implemented")
}

type fakeIdentity struct {
	identity domain.Identity
	err      error
}

func (f fakeIdentity) GetCallerIdentity(context.Context) (domain.Identity, error) {
	return f.identity, f.err
}

func TestListInstancesReturnsContextWarningsAndFilteredInstances(t *testing.T) {
	auth := domain.AuthContext{
		Mode:    domain.AuthModeProfileActive,
		Profile: "dev",
		Region:  "eu-central-1",
	}
	inventory := fakeInventory{
		instances: []domain.Instance{
			{ID: "i-1", Name: "api", State: "running", Region: "eu-central-1", SSMStatus: domain.SSMStatusOnline},
			{ID: "i-2", Name: "old-api", State: "terminated", Region: "eu-central-1", SSMStatus: domain.SSMStatusNotManaged},
			{ID: "i-3", Name: "web", State: "running", Region: "eu-central-1", SSMStatus: domain.SSMStatusOnline},
		},
		warnings: []domain.Warning{{Code: "partial", Message: "SSM failed"}},
	}
	identity := fakeIdentity{identity: domain.Identity{Account: "123456789012", ARN: "arn:aws:sts::123456789012:assumed-role/dev/test"}}

	got, err := ListInstances(context.Background(), auth, inventory, identity, ListFilters{Name: "api"})
	if err != nil {
		t.Fatalf("ListInstances returned error: %v", err)
	}

	if got.Auth != auth || got.Region != auth.Region || got.Account != identity.identity.Account || got.ARN != identity.identity.ARN {
		t.Fatalf("unexpected context metadata: %#v", got)
	}
	if len(got.Warnings) != 1 || got.Warnings[0].Code != "partial" {
		t.Fatalf("expected warning to be preserved, got %#v", got.Warnings)
	}
	if len(got.Instances) != 1 || got.Instances[0].ID != "i-1" {
		t.Fatalf("expected filtered non-terminated api instance, got %#v", got.Instances)
	}
}

func TestListInstancesUsesEmptySlicesForEmptyResult(t *testing.T) {
	auth := domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"}
	result, err := ListInstances(
		context.Background(),
		auth,
		fakeInventory{},
		fakeIdentity{identity: domain.Identity{Account: "123456789012", ARN: "arn"}},
		ListFilters{},
	)
	if err != nil {
		t.Fatalf("ListInstances returned error: %v", err)
	}
	if result.Warnings == nil {
		t.Fatal("expected warnings to be an empty slice, got nil")
	}
	if result.Instances == nil {
		t.Fatal("expected instances to be an empty slice, got nil")
	}
}

func TestListInstancesIdentityErrorIsRuntimeExitError(t *testing.T) {
	_, err := ListInstances(context.Background(), domain.AuthContext{}, fakeInventory{}, fakeIdentity{err: errors.New("sts denied")}, ListFilters{})
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitRuntimeError {
		t.Fatalf("expected runtime ExitError, got %T %[1]v", err)
	}
}

func TestListInstancesInvalidFilterIsUsageExitError(t *testing.T) {
	_, err := ListInstances(context.Background(), domain.AuthContext{}, fakeInventory{}, fakeIdentity{}, ListFilters{SSMStatus: "ready"})
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitUsageError {
		t.Fatalf("expected usage ExitError, got %T %[1]v", err)
	}
}
