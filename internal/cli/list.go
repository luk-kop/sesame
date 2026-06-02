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
	Limit     int
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
			if opts.Limit < 0 {
				return &app.ExitError{Code: app.ExitUsageError, Err: fmt.Errorf("limit must be greater than or equal to 0")}
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

			clients, inventory, identity, _, err := buildProviders(cmd.Context(), global)
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
			if err := writeTableWithOptions(os.Stdout, result, tableOptions{Limit: opts.Limit}); err != nil {
				return &app.ExitError{Code: app.ExitRuntimeError, Err: err}
			}
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
	cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum table rows to print; 0 prints all rows")
	return cmd
}

func writeJSON(w io.Writer, result domain.ListResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func writeTable(w io.Writer, result domain.ListResult) error {
	return writeTableWithOptions(w, result, tableOptions{})
}

type tableOptions struct {
	Limit int
}

func writeTableWithOptions(w io.Writer, result domain.ListResult, opts tableOptions) error {
	if err := writeTableMetadata(w, result, opts); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	return writeInstanceRows(w, result, opts)
}

func writeTableMetadata(w io.Writer, result domain.ListResult, opts tableOptions) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	auth := string(result.Auth.Mode)
	if result.Auth.Mode == domain.AuthModeProfileActive && result.Auth.Profile != "" {
		auth = fmt.Sprintf("%s %s", result.Auth.Mode, result.Auth.Profile)
	}
	if _, err := fmt.Fprintf(tw, "AUTH\t%s\n", auth); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "REGION\t%s\n", empty(result.Region)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "ACCOUNT\t%s\n", empty(result.Account)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "ARN\t%s\n", empty(result.ARN)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "INSTANCES\t%d\n", len(result.Instances)); err != nil {
		return err
	}
	if opts.Limit > 0 && opts.Limit < len(result.Instances) {
		if _, err := fmt.Fprintf(tw, "SHOWN\t%d\n", opts.Limit); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeInstanceRows(w io.Writer, result domain.ListResult, opts tableOptions) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "#\tNAME\tINSTANCE ID\tSTATE\tSSM\tPRIVATE IP\tPUBLIC IP\tREGION\n"); err != nil {
		return err
	}
	instances := result.Instances
	if opts.Limit > 0 && opts.Limit < len(instances) {
		instances = instances[:opts.Limit]
	}
	for i, inst := range instances {
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			i+1,
			trimTableCell(empty(inst.Name), 40),
			inst.ID,
			inst.State,
			inst.SSMStatus,
			empty(inst.PrivateIP),
			empty(inst.PublicIP),
			inst.Region,
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func trimTableCell(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func empty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
