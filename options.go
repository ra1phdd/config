package config

type Options struct {
	// ConfigPath is the main config file path; supported extensions are .env, .json, .yaml, and .yml.
	ConfigPath string
	// SecurityPath is an optional YAML overlay path for secrets and other private values.
	SecurityPath string
	// DotEnvPaths are optional additional .env files loaded before process env parsing.
	DotEnvPaths []string

	// ApplyEnvironment enables parsing values from process environment variables.
	ApplyEnvironment bool
	// LoadDotEnv enables loading DotEnvPaths; no implicit .env path is used.
	LoadDotEnv bool
	// OverrideDotEnv allows DotEnvPaths to overwrite already-set process environment values.
	OverrideDotEnv bool
	// AllowMissing returns defaults when the main config file does not exist.
	AllowMissing bool
	// AllowEmpty returns defaults when the main config file is empty.
	AllowEmpty bool
	// AllowSymlinks permits reading or writing through symlink paths.
	AllowSymlinks bool
	// StrictJSON rejects unknown JSON fields when parsing .json configs.
	StrictJSON bool
	// Validate runs Validate() hooks after loading and before saving.
	Validate bool

	// Resolver resolves path markers and environment-based path overrides.
	Resolver Replacer
}

type Option func(*Options)

func defaultOptions() Options {
	return Options{
		ApplyEnvironment: true,
		AllowMissing:     true,
		AllowEmpty:       true,
		Validate:         true,
		Resolver:         DefaultPathResolver(),
	}
}

func WithConfigPath(path string) Option {
	return func(o *Options) {
		o.ConfigPath = path
	}
}

func WithSecurityPath(path string) Option {
	return func(o *Options) {
		o.SecurityPath = path
	}
}

func WithDotEnv(paths ...string) Option {
	return func(o *Options) {
		o.DotEnvPaths = append([]string(nil), paths...)
		o.LoadDotEnv = true
	}
}

func WithDotEnvEnabled(enabled bool) Option {
	return func(o *Options) {
		o.LoadDotEnv = enabled
	}
}

func WithDotEnvOverride(enabled bool) Option {
	return func(o *Options) {
		o.OverrideDotEnv = enabled
	}
}

func WithEnvironment(enabled bool) Option {
	return func(o *Options) {
		o.ApplyEnvironment = enabled
	}
}

func WithMissingConfigAllowed(allowed bool) Option {
	return func(o *Options) {
		o.AllowMissing = allowed
	}
}

func WithEmptyConfigAllowed(allowed bool) Option {
	return func(o *Options) {
		o.AllowEmpty = allowed
	}
}

func WithSymlinksAllowed(allowed bool) Option {
	return func(o *Options) {
		o.AllowSymlinks = allowed
	}
}

func WithStrictJSON(enabled bool) Option {
	return func(o *Options) {
		o.StrictJSON = enabled
	}
}

func WithValidation(enabled bool) Option {
	return func(o *Options) {
		o.Validate = enabled
	}
}

func WithPathResolver(resolver Replacer) Option {
	return func(o *Options) {
		o.Resolver = resolver.WithDefaults()
	}
}

func WithConfigPathEnv(name string) Option {
	return func(o *Options) {
		o.Resolver.EnvConfig = name
	}
}

func WithSecurityPathEnv(name string) Option {
	return func(o *Options) {
		o.Resolver.EnvSecurity = name
	}
}

func applyOptions(options ...Option) Options {
	opts := defaultOptions()
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	opts.Resolver = opts.Resolver.WithDefaults()
	return opts
}
