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
	loadConfig                          func(envFile string) (config.Config, error)
	runMigrations                       func(dsn, migrationsPath, direction string, steps int) error
	runFolderSiblingUniquenessPreflight func(dsn string) error
}

func defaultMigrateDeps() migrateDeps {
	return migrateDeps{
		loadConfig:                          config.LoadFromEnvFile,
		runMigrations:                       database.RunMigrations,
		runFolderSiblingUniquenessPreflight: database.RunFolderSiblingUniquenessPreflight,
	}
}

func run(args []string, deps migrateDeps) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	envFile := fs.String("env-file", ".env", "environment file to load")
	direction := fs.String("direction", "up", "migration direction: up or down")
	steps := fs.Int("steps", 0, "number of steps for down migration, 0 means all")
	preflight := fs.String("preflight", "", "optional preflight check target")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := deps.loadConfig(*envFile)
	if err != nil {
		return err
	}

	if *preflight != "" {
		switch *preflight {
		case "folder-sibling-uniqueness":
			if deps.runFolderSiblingUniquenessPreflight == nil {
				return fmt.Errorf("folder sibling-name uniqueness preflight dependency is not configured")
			}
			return deps.runFolderSiblingUniquenessPreflight(cfg.PostgresDSN)
		default:
			return fmt.Errorf("unsupported preflight target %q", *preflight)
		}
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
