package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"

	"sesame/internal/domain"
)

type AwsCliStarter struct {
	Auth domain.AuthContext
}

func (s AwsCliStarter) CheckDependencies() error {
	if _, err := exec.LookPath("aws"); err != nil {
		return fmt.Errorf("aws CLI not found in PATH")
	}
	if _, err := exec.LookPath("session-manager-plugin"); err != nil {
		return fmt.Errorf("session-manager-plugin not found in PATH")
	}
	return nil
}

func (s AwsCliStarter) StartShell(ctx context.Context, target domain.Instance) error {
	cmd := exec.CommandContext(ctx, "aws", s.shellArgs(target.ID)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s AwsCliStarter) StartTunnel(ctx context.Context, target domain.Instance, localPort, remotePort int) error {
	if err := validatePort(localPort, "local-port"); err != nil {
		return err
	}
	if err := validatePort(remotePort, "remote-port"); err != nil {
		return err
	}
	if err := checkLocalPortAvailable(localPort); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "aws", s.tunnelArgs(target.ID, localPort, remotePort)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s AwsCliStarter) shellArgs(instanceID string) []string {
	args := []string{"ssm", "start-session", "--target", instanceID, "--region", s.Auth.Region}
	if s.Auth.Mode == domain.AuthModeProfileActive && s.Auth.Profile != "" {
		args = append(args, "--profile", s.Auth.Profile)
	}
	return args
}

func (s AwsCliStarter) tunnelArgs(instanceID string, localPort, remotePort int) []string {
	params, _ := json.Marshal(map[string][]string{
		"portNumber":      {strconv.Itoa(remotePort)},
		"localPortNumber": {strconv.Itoa(localPort)},
	})
	args := []string{
		"ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartPortForwardingSession",
		"--parameters", string(params),
		"--region", s.Auth.Region,
	}
	if s.Auth.Mode == domain.AuthModeProfileActive && s.Auth.Profile != "" {
		args = append(args, "--profile", s.Auth.Profile)
	}
	return args
}

func validatePort(port int, name string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be in range 1..65535", name)
	}
	return nil
}

func checkLocalPortAvailable(port int) error {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("local port %d is not available: %w", port, err)
	}
	if closeErr := ln.Close(); closeErr != nil {
		return errors.Join(err, closeErr)
	}
	return nil
}
