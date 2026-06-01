package main

import (
	"os"

	"sesame/internal/cli"
)

// version, revision, and buildDate are injected at build time via -ldflags.
var version = "dev"
var revision = "unknown"
var buildDate = "unknown"

func main() {
	os.Exit(cli.Execute(cli.BuildInfo{
		Version:   version,
		Revision:  revision,
		BuildDate: buildDate,
	}))
}
