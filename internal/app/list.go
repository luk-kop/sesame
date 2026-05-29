package app

import (
	"context"
	"fmt"

	"sesame/internal/domain"
)

func ListInstances(ctx context.Context, auth domain.AuthContext, inventory InventoryProvider, identity IdentityProvider, filters ListFilters) (domain.ListResult, error) {
	filters, err := NormalizeListFilters(filters)
	if err != nil {
		return domain.ListResult{}, &ExitError{Code: ExitUsageError, Err: err}
	}

	ident, err := identity.GetCallerIdentity(ctx)
	if err != nil {
		return domain.ListResult{}, &ExitError{Code: ExitRuntimeError, Err: fmt.Errorf("get caller identity: %w", err)}
	}

	instances, warnings, err := inventory.ListInstances(ctx)
	if err != nil {
		return domain.ListResult{}, &ExitError{Code: ExitRuntimeError, Err: err}
	}
	if warnings == nil {
		warnings = []domain.Warning{}
	}
	filtered := ApplyListFilters(instances, filters)
	if filtered == nil {
		filtered = []domain.Instance{}
	}

	return domain.ListResult{
		Auth:      auth,
		Region:    auth.Region,
		Account:   ident.Account,
		ARN:       ident.ARN,
		Warnings:  warnings,
		Instances: filtered,
	}, nil
}
