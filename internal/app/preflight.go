package app

import (
	"context"
	"errors"
	"fmt"

	"sesame/internal/domain"
)

type PreflightOptions struct {
	Force bool
}

func PreflightSession(ctx context.Context, inventory InventoryProvider, identity IdentityProvider, instanceID string, opts PreflightOptions) (domain.Instance, domain.Identity, error) {
	ident, err := identity.GetCallerIdentity(ctx)
	if err != nil {
		return domain.Instance{}, domain.Identity{}, &ExitError{Code: ExitRuntimeError, Err: fmt.Errorf("get caller identity: %w", err)}
	}

	inst, err := inventory.GetInstance(ctx, instanceID)
	if err != nil {
		code := ExitRuntimeError
		if errors.Is(err, domain.ErrInstanceNotFound) {
			code = ExitPreflightFailed
		}
		return domain.Instance{}, ident, &ExitError{Code: code, Err: fmt.Errorf("lookup instance %s: %w", instanceID, err)}
	}

	if !opts.Force && inst.SSMStatus != domain.SSMStatusOnline {
		return domain.Instance{}, ident, &ExitError{
			Code: ExitPreflightFailed,
			Err:  fmt.Errorf("instance %s is not SSM online: %s", instanceID, inst.SSMStatus),
		}
	}

	return inst, ident, nil
}
