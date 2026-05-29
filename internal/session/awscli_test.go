package session

import (
	"reflect"
	"testing"

	"sesame/internal/domain"
)

func TestShellArgsProfileActive(t *testing.T) {
	starter := AwsCliStarter{Auth: domain.AuthContext{
		Mode:    domain.AuthModeProfileActive,
		Profile: "dev",
		Region:  "eu-central-1",
	}}

	got := starter.shellArgs("i-123")
	want := []string{"ssm", "start-session", "--target", "i-123", "--region", "eu-central-1", "--profile", "dev"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestShellArgsEnvActiveDoesNotPassProfile(t *testing.T) {
	starter := AwsCliStarter{Auth: domain.AuthContext{
		Mode:   domain.AuthModeEnvActive,
		Region: "eu-central-1",
	}}

	got := starter.shellArgs("i-123")
	want := []string{"ssm", "start-session", "--target", "i-123", "--region", "eu-central-1"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestTunnelArgs(t *testing.T) {
	starter := AwsCliStarter{Auth: domain.AuthContext{
		Mode:    domain.AuthModeProfileActive,
		Profile: "dev",
		Region:  "eu-central-1",
	}}

	got := starter.tunnelArgs("i-123", 15432, 5432)
	want := []string{
		"ssm", "start-session",
		"--target", "i-123",
		"--document-name", "AWS-StartPortForwardingSession",
		"--parameters", `{"localPortNumber":["15432"],"portNumber":["5432"]}`,
		"--region", "eu-central-1",
		"--profile", "dev",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestValidatePort(t *testing.T) {
	for _, port := range []int{1, 65535} {
		if err := ValidatePort(port, "port"); err != nil {
			t.Fatalf("expected port %d to be valid: %v", port, err)
		}
	}
	for _, port := range []int{0, 65536, -1} {
		if err := ValidatePort(port, "port"); err == nil {
			t.Fatalf("expected port %d to be invalid", port)
		}
	}
}
