package cli

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"sesame/internal/app"
	"sesame/internal/session"
)

type sessionOptions struct {
	Force bool
}

type tunnelOptions struct {
	Force      bool
	LocalPort  int
	RemotePort int
}

func newShellCommand(global *globalOptions) *cobra.Command {
	opts := &sessionOptions{}
	cmd := &cobra.Command{
		Use:   "shell <instance-id>",
		Short: "Start an SSM shell session",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return &app.ExitError{Code: app.ExitUsageError, Err: fmt.Errorf("shell requires exactly one instance-id")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionCommand(cmd, global, args[0], opts.Force, func(ctx context.Context, starter session.AwsCliStarter, targetID string) error {
				clients, inventory, identity, err := buildProviders(ctx, global)
				if err != nil {
					return &app.ExitError{Code: app.ExitRuntimeError, Err: err}
				}
				target, _, err := app.PreflightSession(ctx, inventory, identity, targetID, app.PreflightOptions{Force: opts.Force})
				if err != nil {
					return err
				}
				starter.Auth = clients.Auth
				return starter.StartShell(ctx, target)
			})
		},
	}
	cmd.Flags().BoolVar(&opts.Force, "force", false, "skip SSM status block after EC2 lookup succeeds")
	return cmd
}

func newTunnelCommand(global *globalOptions) *cobra.Command {
	opts := &tunnelOptions{}
	cmd := &cobra.Command{
		Use:   "tunnel <instance-id>",
		Short: "Start an SSM port forwarding session in the foreground",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return &app.ExitError{Code: app.ExitUsageError, Err: fmt.Errorf("tunnel requires exactly one instance-id")}
			}
			if opts.LocalPort == 0 || opts.RemotePort == 0 {
				return &app.ExitError{Code: app.ExitUsageError, Err: fmt.Errorf("--local-port and --remote-port are required")}
			}
			if err := session.ValidatePort(opts.LocalPort, "local-port"); err != nil {
				return &app.ExitError{Code: app.ExitUsageError, Err: err}
			}
			if err := session.ValidatePort(opts.RemotePort, "remote-port"); err != nil {
				return &app.ExitError{Code: app.ExitUsageError, Err: err}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionCommand(cmd, global, args[0], opts.Force, func(ctx context.Context, starter session.AwsCliStarter, targetID string) error {
				clients, inventory, identity, err := buildProviders(ctx, global)
				if err != nil {
					return &app.ExitError{Code: app.ExitRuntimeError, Err: err}
				}
				target, _, err := app.PreflightSession(ctx, inventory, identity, targetID, app.PreflightOptions{Force: opts.Force})
				if err != nil {
					return err
				}
				starter.Auth = clients.Auth
				return starter.StartTunnel(ctx, target, opts.LocalPort, opts.RemotePort)
			})
		},
	}
	cmd.Flags().BoolVar(&opts.Force, "force", false, "skip SSM status block after EC2 lookup succeeds")
	cmd.Flags().IntVar(&opts.LocalPort, "local-port", 0, "local port")
	cmd.Flags().IntVar(&opts.RemotePort, "remote-port", 0, "remote port on the instance")
	return cmd
}

func runSessionCommand(cmd *cobra.Command, global *globalOptions, instanceID string, force bool, run func(context.Context, session.AwsCliStarter, string) error) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	starter := session.AwsCliStarter{}
	if err := starter.CheckDependencies(); err != nil {
		return &app.ExitError{Code: app.ExitMissingDependency, Err: err}
	}

	err := run(ctx, starter, instanceID)
	if err == nil {
		return nil
	}
	var exitErr *app.ExitError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	return &app.ExitError{Code: app.ExitRuntimeError, Err: err}
}
