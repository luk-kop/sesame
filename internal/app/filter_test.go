package app

import (
	"testing"

	"sesame/internal/domain"
)

func TestApplyListFiltersDefaultsExcludeTerminated(t *testing.T) {
	instances := []domain.Instance{
		{ID: "i-2", Name: "api", State: "terminated", SSMStatus: domain.SSMStatusOnline},
		{ID: "i-1", Name: "web", State: "running", SSMStatus: domain.SSMStatusOnline},
	}

	got := ApplyListFilters(instances, ListFilters{})

	if len(got) != 1 || got[0].ID != "i-1" {
		t.Fatalf("expected only non-terminated instance, got %#v", got)
	}
}

func TestApplyListFiltersNameSubstringCaseInsensitive(t *testing.T) {
	instances := []domain.Instance{
		{ID: "i-1", Name: "prod-api-worker", State: "running", SSMStatus: domain.SSMStatusOnline},
		{ID: "i-2", Name: "bastion", State: "running", SSMStatus: domain.SSMStatusOnline},
	}

	got := ApplyListFilters(instances, ListFilters{Name: "API"})

	if len(got) != 1 || got[0].ID != "i-1" {
		t.Fatalf("expected substring match by Name tag, got %#v", got)
	}
}
