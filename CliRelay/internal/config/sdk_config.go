// Package config provides configuration management for the CLI Proxy API server.
// It handles loading and parsing YAML configuration files, and provides structured
// access to application settings including server port, authentication directory,
// debug settings, proxy configuration, and API keys.
package config

// SDKConfig represents the application's configuration, loaded from a YAML file.
type SDKConfig struct {
	// ProxyURL is the URL of an optional proxy server to use for outbound requests.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// ForceModelPrefix requires explicit model prefixes (e.g., "teamA/gemini-3-pro-preview")
	// to target prefixed credentials. When false, unprefixed model requests may use prefixed
	// credentials as well.
	ForceModelPrefix bool `yaml:"force-model-prefix" json:"force-model-prefix"`

	// RequestLog enables or disables detailed request logging functionality.
	RequestLog bool `yaml:"request-log" json:"request-log"`

	// UsageLogContentEnabled controls whether detailed request/response content
	// is persisted to SQLite usage logs.
	UsageLogContentEnabled bool `yaml:"usage-log-content-enabled" json:"usage-log-content-enabled"`

	// APIKeys is a list of keys for authenticating clients to this proxy server.
	APIKeys []string `yaml:"api-keys" json:"api-keys"`

	// APIKeyEntries is a list of API key entries with metadata for advanced management.
	// Keys from both APIKeys and APIKeyEntries are valid for authentication, but
	// entries here take precedence over legacy APIKeys when the same key appears in both.
	APIKeyEntries []APIKeyEntry `yaml:"api-key-entries,omitempty" json:"api-key-entries,omitempty"`

	// PassthroughHeaders controls whether upstream response headers are forwarded to downstream clients.
	// Default is false (disabled).
	PassthroughHeaders bool `yaml:"passthrough-headers" json:"passthrough-headers"`

	// Streaming configures server-side streaming behavior (keep-alives and safe bootstrap retries).
	Streaming StreamingConfig `yaml:"streaming" json:"streaming"`

	// NonStreamKeepAliveInterval controls how often blank lines are emitted for non-streaming responses.
	// <= 0 disables keep-alives. Value is in seconds.
	NonStreamKeepAliveInterval int `yaml:"nonstream-keepalive-interval,omitempty" json:"nonstream-keepalive-interval,omitempty"`
}

// StreamingConfig holds server streaming behavior configuration.
type StreamingConfig struct {
	// KeepAliveSeconds controls how often the server emits SSE heartbeats (": keep-alive\n\n").
	// <= 0 disables keep-alives. Default is 0.
	KeepAliveSeconds int `yaml:"keepalive-seconds,omitempty" json:"keepalive-seconds,omitempty"`

	// BootstrapRetries controls how many times the server may retry a streaming request before any bytes are sent,
	// to allow auth rotation / transient recovery.
	// <= 0 disables bootstrap retries. Default is 0.
	BootstrapRetries int `yaml:"bootstrap-retries,omitempty" json:"bootstrap-retries,omitempty"`
}

// APIKeyEntry represents an API key with optional metadata for advanced management.
type APIKeyEntry struct {
	// Key is the API key string used for authentication.
	Key string `yaml:"key" json:"key"`

	// Name is a human-readable label for this key.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Disabled marks this key as inactive. Disabled keys cannot authenticate.
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`

	// DailyLimit is the maximum number of requests per day. 0 means unlimited.
	DailyLimit int `yaml:"daily-limit,omitempty" json:"daily-limit,omitempty"`

	// TotalQuota is the total number of requests allowed. 0 means unlimited.
	TotalQuota int `yaml:"total-quota,omitempty" json:"total-quota,omitempty"`

	// AllowedModels lists model patterns this key can access. Empty means all models.
	AllowedModels []string `yaml:"allowed-models,omitempty" json:"allowed-models,omitempty"`

	// ProviderAccess limits which providers and provider-scoped models this key can access.
	// Empty means all providers are allowed.
	ProviderAccess []APIKeyProviderAccess `yaml:"provider-access,omitempty" json:"provider-access,omitempty"`

	// CreatedAt is the ISO 8601 timestamp when this key was created.
	CreatedAt string `yaml:"created-at,omitempty" json:"created-at,omitempty"`
}

// APIKeyProviderAccess describes a provider-scoped access rule for an API key.
type APIKeyProviderAccess struct {
	// Provider is the provider identifier, for example "gemini" or "openai-compatibility".
	Provider string `yaml:"provider" json:"provider"`

	// Channels limits access to the listed provider instances/auth channels. Empty means all
	// channels under the provider are allowed.
	Channels []string `yaml:"channels,omitempty" json:"channels,omitempty"`

	// Models limits access to the listed models for this provider. Empty means all models
	// under the provider are allowed. This remains for backward compatibility; the management
	// UI now edits global allowed-models separately.
	Models []string `yaml:"models,omitempty" json:"models,omitempty"`
}

// AllAPIKeys returns a merged, deduplicated list of all API key strings
// from both the legacy APIKeys slice and the new APIKeyEntries slice.
func (c *SDKConfig) AllAPIKeys() []string {
	seen := make(map[string]struct{}, len(c.APIKeys)+len(c.APIKeyEntries))
	var keys []string
	for _, k := range c.APIKeys {
		trimmed := k
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		keys = append(keys, trimmed)
	}
	for _, entry := range c.APIKeyEntries {
		trimmed := entry.Key
		if trimmed == "" || entry.Disabled {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		keys = append(keys, trimmed)
	}
	return keys
}
