package main

import (
	"flag"
	"fmt"
	"os"

	"note-app/internal/infrastructure/config"
	"note-app/internal/infrastructure/database"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up or down")
	steps := flag.Int("steps", 0, "number of steps for down migration, 0 means all")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := database.RunMigrations(cfg.PostgresDSN, "migrations", *direction, *steps); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
