package health

import (
	"fmt"
	"os/exec"
)

type DependencyStatus struct {
	AWSCLI               bool
	SessionManagerPlugin bool
}

func CheckSessionDependencies() DependencyStatus {
	_, awsErr := exec.LookPath("aws")
	_, pluginErr := exec.LookPath("session-manager-plugin")
	return DependencyStatus{
		AWSCLI:               awsErr == nil,
		SessionManagerPlugin: pluginErr == nil,
	}
}

func (s DependencyStatus) Error() error {
	if s.AWSCLI && s.SessionManagerPlugin {
		return nil
	}
	if !s.AWSCLI {
		return fmt.Errorf("aws CLI not found in PATH")
	}
	return fmt.Errorf("session-manager-plugin not found in PATH")
}
