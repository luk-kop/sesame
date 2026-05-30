package awsclient

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ListSharedProfiles() []string {
	profiles := map[string]struct{}{}
	for _, profile := range readProfileNames(sharedConfigPath(), true) {
		profiles[profile] = struct{}{}
	}
	for _, profile := range readProfileNames(sharedCredentialsPath(), false) {
		profiles[profile] = struct{}{}
	}
	if len(profiles) == 0 {
		profiles["default"] = struct{}{}
	}

	names := make([]string, 0, len(profiles))
	for profile := range profiles {
		names = append(names, profile)
	}
	sort.Strings(names)
	return names
}

func sharedConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("AWS_CONFIG_FILE")); path != "" {
		return path
	}
	return homeAWSPath("config")
}

func sharedCredentialsPath() string {
	if path := strings.TrimSpace(os.Getenv("AWS_SHARED_CREDENTIALS_FILE")); path != "" {
		return path
	}
	return homeAWSPath("credentials")
}

func homeAWSPath(name string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".aws", name)
}

func readProfileNames(path string, configFile bool) []string {
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var profiles []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[") || !strings.Contains(line, "]") {
			continue
		}
		name := strings.TrimSpace(line[1:strings.Index(line, "]")])
		if name == "" {
			continue
		}
		if configFile {
			switch {
			case name == "default":
				profiles = append(profiles, name)
			case strings.HasPrefix(name, "profile "):
				profile := strings.TrimSpace(strings.TrimPrefix(name, "profile "))
				if profile != "" {
					profiles = append(profiles, profile)
				}
			}
			continue
		}
		profiles = append(profiles, name)
	}
	return profiles
}
