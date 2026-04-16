package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ProviderCredential holds credentials for a single provider.
type ProviderCredential struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url,omitempty"`
}

// Credentials holds all provider credentials from credentials.yml.
type Credentials struct {
	Providers map[string]ProviderCredential `yaml:"providers"`
}

// LoadCredentials reads ~/.pragma/credentials.yml.
// Returns zero Credentials if file does not exist.
func LoadCredentials() (Credentials, error) {
	path, err := CredentialsPath()
	if err != nil {
		return Credentials{}, fmt.Errorf("resolve credentials path: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, nil
		}
		return Credentials{}, fmt.Errorf("read %s: %w", path, err)
	}

	var creds Credentials
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return creds, nil
}

// SaveCredentials writes credentials to ~/.pragma/credentials.yml with 0600 perms.
func SaveCredentials(creds Credentials) error {
	path, err := CredentialsPath()
	if err != nil {
		return fmt.Errorf("resolve credentials path: %w", err)
	}

	dir, dirErr := PragmaHome()
	if dirErr != nil {
		return dirErr
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(creds)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// CredentialFor returns the credential for a specific provider, or zero value.
func (c Credentials) CredentialFor(provider string) ProviderCredential {
	if c.Providers == nil {
		return ProviderCredential{}
	}
	return c.Providers[provider]
}
