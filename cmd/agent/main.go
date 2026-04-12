package main

import (
	"fmt"
	"os"

	"github.com/FrostWalk/backrest-config-backup/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
