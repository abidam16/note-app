package main

import (
	"os"

	"note-app/internal/infrastructure/config"
)

var (
	loadConfigFn  = config.Load
	runFn         = run
	depsFactoryFn = defaultRuntimeDeps
	exitFn        = os.Exit
)

func main() {
	cfg, err := loadConfigFn()
	if err != nil {
		panic(err)
	}

	if err := runFn(cfg, depsFactoryFn()); err != nil {
		exitFn(1)
	}
}
