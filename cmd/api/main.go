package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"note-app/internal/infrastructure/config"
)

var (
	loadConfigFn  = config.LoadFromEnvFile
	runFn         = run
	depsFactoryFn = defaultRuntimeDeps
	exitFn        = os.Exit
	stderrWriter  io.Writer = os.Stderr
)

func parseEnvFile(args []string) (string, error) {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	envFile := fs.String("env-file", ".env", "environment file to load")
	if err := fs.Parse(args); err != nil {
		return "", err
	}

	return *envFile, nil
}

func main() {
	envFile, err := parseEnvFile(os.Args[1:])
	if err != nil {
		exitWithStartupError(err)
		return
	}

	cfg, err := loadConfigFn(envFile)
	if err != nil {
		exitWithStartupError(err)
		return
	}

	if err := runFn(cfg, depsFactoryFn()); err != nil {
		exitWithStartupError(err)
	}
}

func exitWithStartupError(err error) {
	if err != nil {
		_, _ = fmt.Fprintln(stderrWriter, err.Error())
	}
	exitFn(1)
}
