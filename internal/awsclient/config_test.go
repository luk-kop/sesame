package awsclient

import (
	"testing"

	"sesame/internal/domain"
)

func TestResolveAuthContextEnvCredentialsTakePrecedenceOverProfile(t *testing.T) {
	got := ResolveAuthContext(
		ConfigInput{Profile: "cli-profile", Region: "eu-central-1"},
		EnvConfig{
			AccessKeyID:     "AKIA...",
			SecretAccessKey: "secret",
			Profile:         "env-profile",
			Region:          "us-east-1",
		},
	)

	if got.Mode != domain.AuthModeEnvActive {
		t.Fatalf("expected env-active, got %s", got.Mode)
	}
	if got.Profile != "" {
		t.Fatalf("expected profile to be ignored in env-active, got %q", got.Profile)
	}
	if got.Region != "eu-central-1" {
		t.Fatalf("expected explicit input region to win, got %q", got.Region)
	}
}

func TestResolveAuthContextRequiresBothEnvCredentialKeys(t *testing.T) {
	got := ResolveAuthContext(
		ConfigInput{},
		EnvConfig{
			AccessKeyID: "AKIA...",
			Profile:     "dev",
			Region:      "eu-west-1",
		},
	)

	if got.Mode != domain.AuthModeProfileActive {
		t.Fatalf("expected profile-active when secret key is missing, got %s", got.Mode)
	}
	if got.Profile != "dev" {
		t.Fatalf("expected env AWS_PROFILE, got %q", got.Profile)
	}
}

func TestResolveAuthContextProfilePrecedence(t *testing.T) {
	got := ResolveAuthContext(
		ConfigInput{Profile: "flag-profile"},
		EnvConfig{Profile: "env-profile"},
	)

	if got.Mode != domain.AuthModeProfileActive {
		t.Fatalf("expected profile-active, got %s", got.Mode)
	}
	if got.Profile != "flag-profile" {
		t.Fatalf("expected flag profile to win, got %q", got.Profile)
	}
}

func TestResolveAuthContextDefaultProfile(t *testing.T) {
	got := ResolveAuthContext(ConfigInput{}, EnvConfig{})

	if got.Profile != "default" {
		t.Fatalf("expected default profile, got %q", got.Profile)
	}
}

func TestResolveAuthContextRegionPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		input ConfigInput
		env   EnvConfig
		want  string
	}{
		{
			name:  "flag region wins",
			input: ConfigInput{Region: "eu-central-1"},
			env:   EnvConfig{Region: "us-east-1", DefaultRegion: "us-west-2"},
			want:  "eu-central-1",
		},
		{
			name: "AWS_REGION wins over AWS_DEFAULT_REGION",
			env:  EnvConfig{Region: "us-east-1", DefaultRegion: "us-west-2"},
			want: "us-east-1",
		},
		{
			name: "AWS_DEFAULT_REGION fallback",
			env:  EnvConfig{DefaultRegion: "us-west-2"},
			want: "us-west-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAuthContext(tt.input, tt.env)
			if got.Region != tt.want {
				t.Fatalf("expected region %q, got %q", tt.want, got.Region)
			}
		})
	}
}
