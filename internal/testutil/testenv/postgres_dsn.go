package testenv

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const DefaultLocalPostgresDSN = "postgres://noteapp:noteapp@localhost:5432/noteapp?sslmode=disable"

func ResolvePostgresDSN(projectRoot string) (string, error) {
	for _, key := range []string{"TEST_POSTGRES_DSN", "POSTGRES_DSN", "DATABASE_URL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value, nil
		}
	}

	envFiles := make([]string, 0, 2)
	if envFile := strings.TrimSpace(os.Getenv("TEST_ENV_FILE")); envFile != "" {
		if filepath.IsAbs(envFile) {
			envFiles = append(envFiles, envFile)
		} else {
			envFiles = append(envFiles, filepath.Join(projectRoot, envFile))
		}
	}
	envFiles = append(envFiles, filepath.Join(projectRoot, ".env.test"), filepath.Join(projectRoot, ".env"))

	for _, envFile := range envFiles {
		values, err := loadDotEnvValues(envFile)
		if err != nil {
			return "", err
		}
		for _, key := range []string{"TEST_POSTGRES_DSN", "POSTGRES_DSN", "DATABASE_URL"} {
			if value := strings.TrimSpace(values[key]); value != "" {
				return value, nil
			}
		}
	}

	return DefaultLocalPostgresDSN, nil
}

func loadDotEnvValues(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}

		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return values, nil
}
