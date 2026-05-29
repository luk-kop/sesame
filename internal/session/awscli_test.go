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
