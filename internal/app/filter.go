package app

import (
	"fmt"
	"sort"
	"strings"

	"sesame/internal/domain"
)

type ListFilters struct {
	Name      string
	State     string
	SSMStatus string
	AllStates bool
}

var validEC2States = map[string]struct{}{
	"pending":       {},
	"running":       {},
	"shutting-down": {},
	"terminated":    {},
	"stopping":      {},
	"stopped":       {},
}

var validSSMStatuses = map[string]struct{}{
	string(domain.SSMStatusUnknown):        {},
	string(domain.SSMStatusNotManaged):     {},
	string(domain.SSMStatusOnline):         {},
	string(domain.SSMStatusConnectionLost): {},
	string(domain.SSMStatusError):          {},
}

func NormalizeListFilters(filters ListFilters) (ListFilters, error) {
	filters.Name = strings.TrimSpace(filters.Name)
	filters.State = strings.ToLower(strings.TrimSpace(filters.State))
	filters.SSMStatus = strings.ToLower(strings.TrimSpace(filters.SSMStatus))

	if filters.State != "" {
		if _, ok := validEC2States[filters.State]; !ok {
			return ListFilters{}, fmt.Errorf("unsupported EC2 state %q", filters.State)
		}
	}
	if filters.SSMStatus != "" {
		if _, ok := validSSMStatuses[filters.SSMStatus]; !ok {
			return ListFilters{}, fmt.Errorf("unsupported SSM status %q", filters.SSMStatus)
		}
	}

	return filters, nil
}

func ApplyListFilters(instances []domain.Instance, filters ListFilters) []domain.Instance {
	filters, _ = NormalizeListFilters(filters)
	out := make([]domain.Instance, 0, len(instances))
	name := strings.ToLower(filters.Name)
	state := filters.State
	ssmStatus := filters.SSMStatus

	for _, inst := range instances {
		if !filters.AllStates && state == "" && strings.EqualFold(inst.State, "terminated") {
			continue
		}
		if state != "" && !strings.EqualFold(inst.State, state) {
			continue
		}
		if ssmStatus != "" && !strings.EqualFold(string(inst.SSMStatus), ssmStatus) {
			continue
		}
		if name != "" && !strings.Contains(strings.ToLower(inst.Name), name) {
			continue
		}
		out = append(out, inst)
	}

	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].Name
		if left == "" {
			left = out[i].ID
		}
		right := out[j].Name
		if right == "" {
			right = out[j].ID
		}
		if strings.EqualFold(left, right) {
			return out[i].ID < out[j].ID
		}
		return strings.ToLower(left) < strings.ToLower(right)
	})

	return out
}
