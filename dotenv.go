package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func resolveDotEnvPaths(opts Options) ([]string, error) {
	paths := opts.DotEnvPaths
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}

		clean := opts.Resolver.CleanAbs(path)
		if _, err := safeStat(clean, opts.AllowSymlinks); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat .env %s: %w", clean, err)
		}
		resolved = append(resolved, clean)
	}
	return resolved, nil
}

func loadDotEnv(paths []string, override bool) error {
	if len(paths) == 0 {
		return nil
	}
	if override {
		return godotenv.Overload(paths...)
	}
	return godotenv.Load(paths...)
}
