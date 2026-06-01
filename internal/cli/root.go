package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"sesame/internal/app"
	"sesame/internal/awsclient"
	"sesame/internal/domain"
	"sesame/internal/health"
	"sesame/internal/tui"
)

type globalOptions struct {
	Profile string
	Region  string
}

type BuildInfo struct {
	Version   string
	Revision  string
	BuildDate string
}

func Execute(build BuildInfo) int {
	opts := &globalOptions{}
	root := newRootCommand(opts, build)
	if err := root.Execute(); err != nil {
		var exitErr *app.ExitError
		if errors.As(err, &exitErr) {
			fmt.Fprintln(os.Stderr, formatCLIError(exitErr))
			return exitErr.Code
		}
		fmt.Fprintln(os.Stderr, formatCLIError(err))
		return app.ExitRuntimeError
	}
	return app.ExitOK
}

func newRootCommand(opts *globalOptions, build BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sesame",
		Short:         "TUI and CLI for AWS SSM Session Manager",
		Version:       formatBuildInfo(build),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			clients, err := awsclient.NewClients(cmd.Context(), awsclient.ConfigInput{
				Profile: opts.Profile,
				Region:  opts.Region,
			})
			if err != nil {
				return &app.ExitError{Code: app.ExitRuntimeError, Err: err}
			}
			inventory := awsclient.InventoryProvider{
				Region: clients.Auth.Region,
				EC2:    clients.EC2,
				SSM:    clients.SSM,
			}
			identity := awsclient.IdentityProvider{Client: clients.STS}
			regions := awsclient.RegionProvider{EC2: clients.EC2}
			factory := func(ctx context.Context, auth domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
				clients, inventory, identity, regions, err := buildProviders(ctx, &globalOptions{
					Profile: auth.Profile,
					Region:  auth.Region,
				})
				if err != nil {
					return domain.AuthContext{}, nil, nil, nil, err
				}
				return clients.Auth, inventory, identity, regions, nil
			}
			program := tea.NewProgram(tui.NewModelWithProviderFactoryAndVersion(clients.Auth, inventory, identity, regions, health.CheckSessionDependencies(), factory, awsclient.ListSharedProfiles(), formatBuildInfo(build)))
			_, err = program.Run()
			if err != nil {
				return &app.ExitError{Code: app.ExitRuntimeError, Err: err}
			}
			return nil
		},
	}
	cmd.SetVersionTemplate("{{.Version}}\n")

	cmd.PersistentFlags().StringVar(&opts.Profile, "profile", "", "AWS profile to use when env credentials are not active")
	cmd.PersistentFlags().StringVar(&opts.Region, "region", "", "AWS region")
	cmd.SetContext(context.Background())

	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newShellCommand(opts))
	cmd.AddCommand(newTunnelCommand(opts))
	return cmd
}

func formatBuildInfo(build BuildInfo) string {
	return fmt.Sprintf("%s revision=%s build_date=%s", defaultText(build.Version, "dev"), defaultText(build.Revision, "unknown"), defaultText(build.BuildDate, "unknown"))
}

func defaultText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func buildProviders(ctx context.Context, opts *globalOptions) (*awsclient.Clients, awsclient.InventoryProvider, awsclient.IdentityProvider, awsclient.RegionProvider, error) {
	clients, err := awsclient.NewClients(ctx, awsclient.ConfigInput{
		Profile: opts.Profile,
		Region:  opts.Region,
	})
	if err != nil {
		return nil, awsclient.InventoryProvider{}, awsclient.IdentityProvider{}, awsclient.RegionProvider{}, err
	}
	inventory := awsclient.InventoryProvider{
		Region: clients.Auth.Region,
		EC2:    clients.EC2,
		SSM:    clients.SSM,
	}
	identity := awsclient.IdentityProvider{Client: clients.STS}
	regions := awsclient.RegionProvider{EC2: clients.EC2}
	return clients, inventory, identity, regions, nil
}
