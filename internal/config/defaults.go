package config

// Default returns a fresh Config populated with safe defaults.
// First-run setup mutates this and saves it; until a provider is configured,
// Default.Provider and Default.Model are empty strings.
func Default() *Config {
	return &Config{
		Default: DefaultConfig{
			Provider: "",
			Model:    "",
		},
		Providers: map[string]ProviderConfig{},
		Behavior: BehaviorConfig{
			TrustMode:            false,
			AutoCompactThreshold: 80,
			MaxInputRows:         10,

			BackgroundMaxConcurrent:   4,
			BackgroundMaxDepth:        2,
			BackgroundMaxTotal:        32,
			BackgroundDefaultProvider: "",
			BackgroundDefaultModel:    "",
		},
	}
}
