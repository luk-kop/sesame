package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestFormatCLIErrorMakesIMDSCredentialErrorFriendly(t *testing.T) {
	err := errors.New("get caller identity: operation error STS: GetCallerIdentity, get identity: get credentials: failed to refresh cached credentials, no EC2 IMDS role found, operation error ec2imds: GetMetadata, request canceled, context deadline exceeded")

	got := formatCLIError(err)
	for _, want := range []string{
		"AWS credentials unavailable.",
		"No EC2 IMDS role found.",
		"--profile",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected formatted error to contain %q, got %q", want, got)
		}
	}
	for _, notWant := range []string{
		"operation error STS",
		"failed to refresh cached credentials",
		"context deadline exceeded",
	} {
		if strings.Contains(got, notWant) {
			t.Fatalf("expected formatted error to hide %q, got %q", notWant, got)
		}
	}
}

func TestFormatCLIErrorCleansAWSOperationPrefixes(t *testing.T) {
	err := errors.New("operation error EC2: DescribeInstances, api error UnauthorizedOperation: denied")

	got := formatCLIError(err)
	if strings.Contains(got, "operation error EC2") || strings.Contains(got, "DescribeInstances") {
		t.Fatalf("expected noisy operation prefix to be removed, got %q", got)
	}
	if !strings.Contains(got, "api error UnauthorizedOperation: denied") {
		t.Fatalf("expected underlying error to remain, got %q", got)
	}
}
