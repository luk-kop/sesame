package main

import (
	"os"

	"sesame/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
