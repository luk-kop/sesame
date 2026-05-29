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

func TestApplyListFiltersStateIncludesTerminatedWhenExplicit(t *testing.T) {
	instances := []domain.Instance{
		{ID: "i-1", Name: "old", State: "terminated", SSMStatus: domain.SSMStatusNotManaged},
		{ID: "i-2", Name: "new", State: "running", SSMStatus: domain.SSMStatusOnline},
	}

	got := ApplyListFilters(instances, ListFilters{State: "terminated"})

	if len(got) != 1 || got[0].ID != "i-1" {
		t.Fatalf("expected explicit terminated state to be included, got %#v", got)
	}
}

func TestApplyListFiltersSSMStatus(t *testing.T) {
	instances := []domain.Instance{
		{ID: "i-1", Name: "managed", State: "running", SSMStatus: domain.SSMStatusOnline},
		{ID: "i-2", Name: "lost", State: "running", SSMStatus: domain.SSMStatusConnectionLost},
	}

	got := ApplyListFilters(instances, ListFilters{SSMStatus: "CONNECTION-LOST"})

	if len(got) != 1 || got[0].ID != "i-2" {
		t.Fatalf("expected SSM status filter to be case-insensitive, got %#v", got)
	}
}

func TestNormalizeListFiltersRejectsInvalidState(t *testing.T) {
	_, err := NormalizeListFilters(ListFilters{State: "deleted"})
	if err == nil {
		t.Fatal("expected invalid state error")
	}
}

func TestNormalizeListFiltersRejectsInvalidSSMStatus(t *testing.T) {
	_, err := NormalizeListFilters(ListFilters{SSMStatus: "ready"})
	if err == nil {
		t.Fatal("expected invalid SSM status error")
	}
}
