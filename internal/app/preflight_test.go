package app

import (
	"context"
	"errors"
	"testing"

	"sesame/internal/domain"
)

type fakePreflightInventory struct {
	instance domain.Instance
	err      error
}

func (f fakePreflightInventory) ListInstances(context.Context) ([]domain.Instance, []domain.Warning, error) {
	return nil, nil, errors.New("not implemented")
}

func (f fakePreflightInventory) GetInstance(context.Context, string) (domain.Instance, error) {
	return f.instance, f.err
}

func TestPreflightSessionRequiresOnlineWithoutForce(t *testing.T) {
	_, _, err := PreflightSession(
		context.Background(),
		fakePreflightInventory{instance: domain.Instance{ID: "i-1", SSMStatus: domain.SSMStatusConnectionLost}},
		fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}},
		"i-1",
		PreflightOptions{},
	)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitPreflightFailed {
		t.Fatalf("expected preflight failure, got %T %[1]v", err)
	}
}

func TestPreflightSessionForceAllowsNonOnlineAfterEC2Lookup(t *testing.T) {
	instance := domain.Instance{ID: "i-1", SSMStatus: domain.SSMStatusError}

	got, _, err := PreflightSession(
		context.Background(),
		fakePreflightInventory{instance: instance},
		fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}},
		"i-1",
		PreflightOptions{Force: true},
	)
	if err != nil {
		t.Fatalf("expected force to allow non-online status, got %v", err)
	}
	if got.ID != "i-1" {
		t.Fatalf("expected instance to be returned, got %#v", got)
	}
}

func TestPreflightSessionInstanceNotFoundIsPreflightFailure(t *testing.T) {
	_, _, err := PreflightSession(
		context.Background(),
		fakePreflightInventory{err: domain.ErrInstanceNotFound},
		fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}},
		"i-missing",
		PreflightOptions{Force: true},
	)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitPreflightFailed {
		t.Fatalf("expected instance not found to be preflight failure, got %T %[1]v", err)
	}
}

func TestPreflightSessionIdentityErrorIsRuntime(t *testing.T) {
	_, _, err := PreflightSession(
		context.Background(),
		fakePreflightInventory{instance: domain.Instance{ID: "i-1", SSMStatus: domain.SSMStatusOnline}},
		fakeIdentity{err: errors.New("sts denied")},
		"i-1",
		PreflightOptions{},
	)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitRuntimeError {
		t.Fatalf("expected identity error to be runtime failure, got %T %[1]v", err)
	}
}
