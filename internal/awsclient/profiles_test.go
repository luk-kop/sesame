package awsclient

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListSharedProfilesReadsAWSConfiguredFiles(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config")
	credentials := filepath.Join(dir, "credentials")
	if err := os.WriteFile(config, []byte(`
[default]
region = eu-west-1
[profile dev]
region = eu-central-1
[profile prod]
role_arn = arn
[sso-session corp]
sso_region = eu-west-1
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentials, []byte(`
[default]
aws_access_key_id = redacted
[legacy]
aws_access_key_id = redacted
[dev]
aws_access_key_id = redacted
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", config)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentials)

	got := ListSharedProfiles()
	want := []string{"default", "dev", "legacy", "prod"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected profiles %#v, got %#v", want, got)
	}
}

func TestListSharedProfilesFallsBackToDefault(t *testing.T) {
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "missing-config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "missing-credentials"))

	got := ListSharedProfiles()
	want := []string{"default"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected fallback profiles %#v, got %#v", want, got)
	}
}
