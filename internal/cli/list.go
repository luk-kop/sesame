package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"sesame/internal/app"
	"sesame/internal/domain"
)

type listOptions struct {
	Output    string
	Name      string
	State     string
	SSMStatus string
	AllStates bool
}

func newListCommand(global *globalOptions) *cobra.Command {
	opts := &listOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List EC2 instances with SSM status",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Output = strings.ToLower(strings.TrimSpace(opts.Output))
			if opts.Output != "table" && opts.Output != "json" {
				return &app.ExitError{Code: app.ExitUsageError, Err: fmt.Errorf("unsupported output: %s", opts.Output)}
			}
			filters, err := app.NormalizeListFilters(app.ListFilters{
				Name:      opts.Name,
				State:     opts.State,
				SSMStatus: opts.SSMStatus,
				AllStates: opts.AllStates,
			})
			if err != nil {
				return &app.ExitError{Code: app.ExitUsageError, Err: err}
			}

			clients, inventory, identity, err := buildProviders(cmd.Context(), global)
			if err != nil {
				return &app.ExitError{Code: app.ExitRuntimeError, Err: err}
			}
			result, err := app.ListInstances(cmd.Context(), clients.Auth, inventory, identity, filters)
			if err != nil {
				return err
			}

			if opts.Output == "json" {
				return writeJSON(os.Stdout, result)
			}
			writeTable(os.Stdout, result)
			for _, warning := range result.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s: %s\n", warning.Code, warning.Message)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Output, "output", "table", "output format: table or json")
	cmd.Flags().StringVar(&opts.Name, "name", "", "case-insensitive substring filter for the Name tag")
	cmd.Flags().StringVar(&opts.State, "state", "", "EC2 state filter: pending, running, shutting-down, terminated, stopping, stopped")
	cmd.Flags().StringVar(&opts.SSMStatus, "ssm", "", "SSM status filter: online, not-managed, connection-lost, unknown, error")
	cmd.Flags().BoolVar(&opts.AllStates, "all-states", false, "include terminated instances")
	return cmd
}

func writeJSON(w io.Writer, result domain.ListResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func writeTable(w io.Writer, result domain.ListResult) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	auth := string(result.Auth.Mode)
	if result.Auth.Mode == domain.AuthModeProfileActive && result.Auth.Profile != "" {
		auth = fmt.Sprintf("%s %s", result.Auth.Mode, result.Auth.Profile)
	}
	fmt.Fprintf(tw, "AUTH\t%s\n", auth)
	fmt.Fprintf(tw, "REGION\t%s\n", empty(result.Region))
	fmt.Fprintf(tw, "ACCOUNT\t%s\n", empty(result.Account))
	fmt.Fprintf(tw, "ARN\t%s\n\n", empty(result.ARN))
	fmt.Fprintf(tw, "NAME\tINSTANCE ID\tSTATE\tSSM\tPRIVATE IP\tPUBLIC IP\tREGION\n")
	for _, inst := range result.Instances {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			empty(inst.Name),
			inst.ID,
			inst.State,
			inst.SSMStatus,
			empty(inst.PrivateIP),
			empty(inst.PublicIP),
			inst.Region,
		)
	}
	_ = tw.Flush()
}

func empty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
