package app

import (
	"context"
	"fmt"

	"sesame/internal/domain"
)

func ListInstances(ctx context.Context, auth domain.AuthContext, inventory InventoryProvider, identity IdentityProvider, filters ListFilters) (domain.ListResult, error) {
	ident, err := identity.GetCallerIdentity(ctx)
	if err != nil {
		return domain.ListResult{}, &ExitError{Code: ExitRuntimeError, Err: fmt.Errorf("get caller identity: %w", err)}
	}

	instances, warnings, err := inventory.ListInstances(ctx)
	if err != nil {
		return domain.ListResult{}, &ExitError{Code: ExitRuntimeError, Err: err}
	}

	return domain.ListResult{
		Auth:      auth,
		Region:    auth.Region,
		Account:   ident.Account,
		ARN:       ident.ARN,
		Warnings:  warnings,
		Instances: ApplyListFilters(instances, filters),
	}, nil
}
