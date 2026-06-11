package config

import (
	"os"
	"path/filepath"
	"strings"
)

type PathResolver struct {
	Getenv      func(string) string
	Getwd       func() (string, error)
	UserHomeDir func() (string, error)
	TempDir     func() string

	EnvConfig   string
	EnvSecurity string
}

type Replacer = PathResolver

func DefaultPathResolver() PathResolver {
	return PathResolver{
		Getenv:      os.Getenv,
		Getwd:       os.Getwd,
		UserHomeDir: os.UserHomeDir,
		TempDir:     os.TempDir,
		EnvConfig:   EnvConfig,
		EnvSecurity: EnvSecurity,
	}
}

func DefaultReplacer() PathResolver {
	return DefaultPathResolver()
}

func (r PathResolver) WithDefaults() PathResolver {
	if r.Getenv == nil {
		r.Getenv = os.Getenv
	}
	if r.Getwd == nil {
		r.Getwd = os.Getwd
	}
	if r.UserHomeDir == nil {
		r.UserHomeDir = os.UserHomeDir
	}
	if r.TempDir == nil {
		r.TempDir = os.TempDir
	}
	if r.EnvConfig == "" {
		r.EnvConfig = EnvConfig
	}
	if r.EnvSecurity == "" {
		r.EnvSecurity = EnvSecurity
	}
	return r
}

func (r PathResolver) ConfigPath(path string) string {
	return r.requiredPath(path, r.EnvConfig, "config")
}

func (r PathResolver) SecurityPath(path string) string {
	return r.requiredPath(path, r.EnvSecurity, "security")

}

func (r PathResolver) requiredPath(path string, envName string, kind string) string {
	resolved := r.preferredPath(path, envName)
	if resolved == "" {
		panic("config: missing " + kind + " path")
	}
	return resolved

}

func (r PathResolver) preferredPath(path string, envName string) string {
	if envName != "" {
		if envPath := strings.TrimSpace(r.Getenv(envName)); envPath != "" {
			return r.CleanAbs(envPath)
		}
	}
	return r.CleanAbs(path)
}

func (r PathResolver) CleanAbs(path string) string {
	return r.ResolveAgainst(path, "")
}

func (r PathResolver) ResolveAgainst(path string, baseDir string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	expanded := r.ExpandMarkers(path)
	if !filepath.IsAbs(expanded) {
		if strings.TrimSpace(baseDir) != "" {
			expanded = filepath.Join(baseDir, expanded)
		} else {
			abs, err := filepath.Abs(expanded)
			if err != nil {
				return filepath.Clean(expanded)
			}
			expanded = abs
		}
	}

	return filepath.Clean(expanded)
}

func (r PathResolver) ExpandMarkers(path string) string {
	home, err := r.UserHomeDir()
	if err != nil {
		home = "."
	}

	cwd, err := r.Getwd()
	if err != nil {
		cwd = "."
	}

	tmpDir := "."
	if r.TempDir != nil {
		tmpDir = r.TempDir()
	}

	replacer := strings.NewReplacer(
		"{HOME}", home,
		"{PWD}", cwd,
		"{CWD}", cwd,
		"{TMP}", tmpDir,
		"{TEMP}", tmpDir,
	)
	return replacer.Replace(path)
}

func ResolvePath(path string) string {
	return DefaultPathResolver().CleanAbs(path)
}
