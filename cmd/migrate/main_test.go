package main

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"note-app/internal/infrastructure/config"
)

func TestRunUsesDefaultFlags(t *testing.T) {
	called := false
	deps := migrateDeps{
		loadConfig: func(envFile string) (config.Config, error) {
			if envFile != ".env" {
				t.Fatalf("unexpected default env file: %s", envFile)
			}
			return config.Config{PostgresDSN: "postgres://example"}, nil
		},
		runMigrations: func(dsn, migrationsPath, direction string, steps int) error {
			called = true
			if dsn != "postgres://example" {
				t.Fatalf("unexpected dsn: %s", dsn)
			}
			if migrationsPath != "migrations" {
				t.Fatalf("unexpected migrations path: %s", migrationsPath)
			}
			if direction != "up" || steps != 0 {
				t.Fatalf("unexpected defaults direction=%s steps=%d", direction, steps)
			}
			return nil
		},
	}

	if err := run(nil, deps); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !called {
		t.Fatal("expected runMigrations to be called")
	}
}

func TestRunParsesFlags(t *testing.T) {
	deps := migrateDeps{
		loadConfig: func(envFile string) (config.Config, error) {
			if envFile != ".env.test" {
				t.Fatalf("unexpected env file: %s", envFile)
			}
			return config.Config{PostgresDSN: "postgres://example"}, nil
		},
		runMigrations: func(_ string, _ string, direction string, steps int) error {
			if direction != "down" || steps != 3 {
				t.Fatalf("unexpected parsed values direction=%s steps=%d", direction, steps)
			}
			return nil
		},
	}

	if err := run([]string{"-env-file", ".env.test", "-direction", "down", "-steps", "3"}, deps); err != nil {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRunPreflight(t *testing.T) {
	migrationCalled := false
	preflightCalled := false
	deps := migrateDeps{
		loadConfig: func(string) (config.Config, error) {
			return config.Config{PostgresDSN: "postgres://example"}, nil
		},
		runMigrations: func(string, string, string, int) error {
			migrationCalled = true
			return nil
		},
		runFolderSiblingUniquenessPreflight: func(dsn string) error {
			preflightCalled = true
			if dsn != "postgres://example" {
				t.Fatalf("unexpected dsn: %s", dsn)
			}
			return nil
		},
	}

	if err := run([]string{"-preflight", "folder-sibling-uniqueness"}, deps); err != nil {
		t.Fatalf("run() preflight error = %v", err)
	}
	if !preflightCalled {
		t.Fatal("expected preflight to be called")
	}
	if migrationCalled {
		t.Fatal("did not expect migrations to run during preflight")
	}
}

func TestRunFlagParseError(t *testing.T) {
	deps := migrateDeps{
		loadConfig: func(string) (config.Config, error) {
			return config.Config{}, nil
		},
		runMigrations: func(string, string, string, int) error {
			return nil
		},
	}

	if err := run([]string{"-steps", "abc"}, deps); err == nil {
		t.Fatal("expected flag parse error")
	}
}

func TestRunUnsupportedPreflightAndMissingDependency(t *testing.T) {
	deps := migrateDeps{
		loadConfig: func(string) (config.Config, error) {
			return config.Config{PostgresDSN: "postgres://example"}, nil
		},
		runMigrations: func(string, string, string, int) error {
			return nil
		},
	}

	if err := run([]string{"-preflight", "unknown"}, deps); err == nil {
		t.Fatal("expected unsupported preflight error")
	}
	if err := run([]string{"-preflight", "folder-sibling-uniqueness"}, deps); err == nil {
		t.Fatal("expected missing preflight dependency error")
	}
}

func TestRunConfigAndMigrationErrors(t *testing.T) {
	cfgErr := errors.New("config failed")
	deps := migrateDeps{
		loadConfig: func(string) (config.Config, error) {
			return config.Config{}, cfgErr
		},
		runMigrations: func(string, string, string, int) error {
			return nil
		},
	}
	if err := run(nil, deps); !errors.Is(err, cfgErr) {
		t.Fatalf("expected config error, got %v", err)
	}

	migErr := errors.New("migrate failed")
	deps = migrateDeps{
		loadConfig: func(string) (config.Config, error) {
			return config.Config{PostgresDSN: "postgres://example"}, nil
		},
		runMigrations: func(string, string, string, int) error {
			return migErr
		},
	}
	if err := run(nil, deps); !errors.Is(err, migErr) {
		t.Fatalf("expected migration error, got %v", err)
	}

	preflightErr := errors.New("preflight failed")
	deps = migrateDeps{
		loadConfig: func(string) (config.Config, error) {
			return config.Config{PostgresDSN: "postgres://example"}, nil
		},
		runFolderSiblingUniquenessPreflight: func(string) error {
			return preflightErr
		},
	}
	if err := run([]string{"-preflight", "folder-sibling-uniqueness"}, deps); !errors.Is(err, preflightErr) {
		t.Fatalf("expected preflight error, got %v", err)
	}
}

func TestDefaultMigrateDeps(t *testing.T) {
	deps := defaultMigrateDeps()
	if deps.loadConfig == nil || deps.runMigrations == nil || deps.runFolderSiblingUniquenessPreflight == nil {
		t.Fatal("expected default migrate deps to be initialized")
	}
}

func TestMainBehavior(t *testing.T) {
	originalRun := runMigrateFn
	originalDepsFactory := depsFactoryFn
	originalExit := exitMigrateFn
	originalStderr := stderrWriter
	originalArgs := os.Args
	defer func() {
		runMigrateFn = originalRun
		depsFactoryFn = originalDepsFactory
		exitMigrateFn = originalExit
		stderrWriter = originalStderr
		os.Args = originalArgs
	}()

	var exitedWith int
	exitMigrateFn = func(code int) { exitedWith = code }
	depsFactoryFn = func() migrateDeps {
		return migrateDeps{}
	}

	stderr := &bytes.Buffer{}
	stderrWriter = stderr

	runMigrateFn = func([]string, migrateDeps) error { return nil }
	os.Args = []string{"migrate"}
	main()
	if exitedWith != 0 {
		t.Fatalf("expected no exit on success, got %d", exitedWith)
	}

	runMigrateFn = func([]string, migrateDeps) error { return errors.New("boom") }
	main()
	if exitedWith != 1 {
		t.Fatalf("expected exit code 1 on error, got %d", exitedWith)
	}
	if stderr.Len() == 0 {
		t.Fatal("expected error to be written to stderr")
	}
}
