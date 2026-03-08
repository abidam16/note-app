package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"note-app/internal/infrastructure/config"
	"note-app/internal/infrastructure/database"
)

type migrateDeps struct {
	loadConfig    func() (config.Config, error)
	runMigrations func(dsn, migrationsPath, direction string, steps int) error
}

func defaultMigrateDeps() migrateDeps {
	return migrateDeps{
		loadConfig:    config.Load,
		runMigrations: database.RunMigrations,
	}
}

func run(args []string, deps migrateDeps) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	direction := fs.String("direction", "up", "migration direction: up or down")
	steps := fs.Int("steps", 0, "number of steps for down migration, 0 means all")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := deps.loadConfig()
	if err != nil {
		return err
	}

	if err := deps.runMigrations(cfg.PostgresDSN, "migrations", *direction, *steps); err != nil {
		return err
	}

	return nil
}

var (
	runMigrateFn            = run
	depsFactoryFn           = defaultMigrateDeps
	exitMigrateFn           = os.Exit
	stderrWriter  io.Writer = os.Stderr
)

func main() {
	if err := runMigrateFn(os.Args[1:], depsFactoryFn()); err != nil {
		fmt.Fprintln(stderrWriter, err)
		exitMigrateFn(1)
	}
}
