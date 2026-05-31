package app

import (
	"context"

	"sesame/internal/domain"
)

type InventoryProvider interface {
	ListInstances(ctx context.Context) ([]domain.Instance, []domain.Warning, error)
	GetInstance(ctx context.Context, instanceID string) (domain.Instance, error)
}

type IdentityProvider interface {
	GetCallerIdentity(ctx context.Context) (domain.Identity, error)
}

type RegionProvider interface {
	ListRegions(ctx context.Context) ([]string, error)
}

type SessionStarter interface {
	CheckDependencies() error
	StartShell(ctx context.Context, target domain.Instance) error
	StartTunnel(ctx context.Context, target domain.Instance, localPort, remotePort int) error
}
