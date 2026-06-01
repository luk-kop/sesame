package cli

import (
	"strings"
)

func formatCLIError(err error) string {
	if err == nil {
		return "unknown error"
	}

	raw := strings.TrimSpace(err.Error())
	if strings.Contains(raw, "failed to refresh cached credentials") && strings.Contains(raw, "no EC2 IMDS role found") {
		return "AWS credentials unavailable. No EC2 IMDS role found.\nCheck credentials, run aws sso login if using SSO, or choose another profile with --profile."
	}
	return cleanupCLIError(raw)
}

func cleanupCLIError(value string) string {
	value = strings.TrimSpace(value)
	replacements := []string{
		"operation error STS: GetCallerIdentity, ",
		"operation error EC2: DescribeInstances, ",
		"operation error SSM: DescribeInstanceInformation, ",
	}
	for _, old := range replacements {
		value = strings.ReplaceAll(value, old, "")
	}
	return value
}
