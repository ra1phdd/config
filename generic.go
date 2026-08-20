package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	env "github.com/caarlos0/env/v11"
)

type ConfigValidator interface {
	Validate() error
}

type Loader struct {
	options      Options
	configPath   string
	securityPath string
}

func LoadGeneric[T any](defaults func() *T, options ...Option) (*T, error) {
	cfg := new(T)
	if defaults != nil {
		if def := defaults(); def != nil {
			cfg = def
		}
	}

	loader, err := NewLoader(options...)
	if err != nil {
		return nil, err
	}

	if err := loader.LoadInto(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func SaveGeneric[T any](cfg *T, options ...Option) error {
	if cfg == nil {
		return ErrNilConfig
	}

	loader, err := NewLoader(options...)
	if err != nil {
		return err
	}

	return loader.Save(cfg)
}

func NewLoader(options ...Option) (*Loader, error) {
	opts := applyOptions(options...)
	configPath := opts.Resolver.ConfigPath(opts.ConfigPath)
	securityPath := opts.Resolver.SecurityPath(opts.SecurityPath)

	return &Loader{
		options:      opts,
		configPath:   configPath,
		securityPath: securityPath,
	}, nil
}

func (l *Loader) ConfigPath() string {
	return l.configPath
}

func (l *Loader) SecurityPath() string {
	return l.securityPath
}

func (l *Loader) LoadInto(target any) error {
	if target == nil {
		return ErrNilConfig
	}

	setCredentialResolver(filepath.Dir(l.configPath), l.options.AllowSymlinks)

	if err := l.loadConfigFile(target); err != nil {
		return err
	}
	if err := bindDirectoryFileMaps(target, l.configPath, l.options.AllowSymlinks, l.options.StrictJSON, l.options.Resolver); err != nil {
		return err
	}

	if err := loadYAMLOverlay(target, l.securityPath, l.options.AllowSymlinks); err != nil {
		return err
	}

	if err := l.applyEnvironment(target); err != nil {
		return err
	}

	return l.validate(target)
}

func (l *Loader) Save(cfg any) error {
	if cfg == nil {
		return ErrNilConfig
	}
	if err := bindUnboundDirectoryFileMaps(cfg, l.configPath, l.options.AllowSymlinks, l.options.StrictJSON, l.options.Resolver); err != nil {
		return err
	}

	if err := l.validate(cfg); err != nil {
		return err
	}

	if err := saveYAMLOverlay(l.securityPath, cfg, l.options.AllowSymlinks); err != nil {
		return fmt.Errorf("save security config: %w", err)
	}

	data, err := encodeConfigWithRefs(l.configPath, cfg, l.options.AllowSymlinks, l.options.Resolver)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return writePrivateFile(l.configPath, data, l.options.AllowSymlinks)
}

func (l *Loader) loadConfigFile(target any) error {
	data, err := readRegularFile(l.configPath, l.options.AllowSymlinks)
	if err != nil {
		if os.IsNotExist(err) && l.options.AllowMissing {
			return nil
		}
		return fmt.Errorf("read config: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		if l.options.AllowEmpty {
			return nil
		}
		return fmt.Errorf("%w: config file is empty", ErrInvalidConfig)
	}

	return loadConfigWithRefs(
		l.configPath,
		data,
		target,
		l.options.AllowSymlinks,
		l.options.StrictJSON,
		l.options.Resolver,
	)
}

func (l *Loader) applyEnvironment(target any) error {
	if !l.options.ApplyEnvironment {
		return nil
	}

	if l.options.LoadDotEnv {
		paths, err := resolveDotEnvPaths(l.options)
		if err != nil {
			return err
		}

		if err := loadDotEnv(paths, l.options.OverrideDotEnv); err != nil {
			return err
		}
	}

	if err := env.Parse(target); err != nil {
		return fmt.Errorf("apply environment: %w", err)
	}

	return nil
}

func (l *Loader) validate(cfg any) error {
	if !l.options.Validate {
		return nil
	}

	validator, ok := cfg.(ConfigValidator)
	if !ok {
		return nil
	}

	if err := validator.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}

	return nil
}
