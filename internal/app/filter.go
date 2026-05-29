package app

import (
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

func ApplyListFilters(instances []domain.Instance, filters ListFilters) []domain.Instance {
	out := make([]domain.Instance, 0, len(instances))
	name := strings.ToLower(filters.Name)
	state := strings.ToLower(filters.State)
	ssmStatus := strings.ToLower(filters.SSMStatus)

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
