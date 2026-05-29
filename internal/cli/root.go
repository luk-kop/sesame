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
	"sesame/internal/tui"
)

type globalOptions struct {
	Profile string
	Region  string
}

func Execute() int {
	opts := &globalOptions{}
	root := newRootCommand(opts)
	if err := root.Execute(); err != nil {
		var exitErr *app.ExitError
		if errors.As(err, &exitErr) {
			fmt.Fprintln(os.Stderr, exitErr.Error())
			return exitErr.Code
		}
		fmt.Fprintln(os.Stderr, err)
		return app.ExitRuntimeError
	}
	return app.ExitOK
}

func newRootCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sesame",
		Short:         "TUI and CLI for AWS SSM Session Manager",
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
			program := tea.NewProgram(tui.NewModel(clients.Auth))
			_, err = program.Run()
			if err != nil {
				return &app.ExitError{Code: app.ExitRuntimeError, Err: err}
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.Profile, "profile", "", "AWS profile to use when env credentials are not active")
	cmd.PersistentFlags().StringVar(&opts.Region, "region", "", "AWS region")
	cmd.SetContext(context.Background())

	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newShellCommand(opts))
	cmd.AddCommand(newTunnelCommand(opts))
	return cmd
}

func buildProviders(ctx context.Context, opts *globalOptions) (*awsclient.Clients, awsclient.InventoryProvider, awsclient.IdentityProvider, error) {
	clients, err := awsclient.NewClients(ctx, awsclient.ConfigInput{
		Profile: opts.Profile,
		Region:  opts.Region,
	})
	if err != nil {
		return nil, awsclient.InventoryProvider{}, awsclient.IdentityProvider{}, err
	}
	inventory := awsclient.InventoryProvider{
		Region: clients.Auth.Region,
		EC2:    clients.EC2,
		SSM:    clients.SSM,
	}
	identity := awsclient.IdentityProvider{Client: clients.STS}
	return clients, inventory, identity, nil
}
