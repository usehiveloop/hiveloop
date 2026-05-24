package integrations

import (
	"fmt"
	"os"
	"strings"

	"github.com/usehivy/hivy/internal/nango"
)

type skippedIntegration struct {
	reason string
}

func (e skippedIntegration) Error() string { return e.reason }

func credentialsFromManifest(m Manifest, provider nango.Provider) (*nango.Credentials, error) {
	if m.Credentials == nil {
		if credentialsRequired(provider.AuthMode) {
			if m.Required {
				return nil, fmt.Errorf("%s requires credentials for %s auth mode", m.ID, provider.AuthMode)
			}
			return nil, skippedIntegration{reason: "missing optional credentials"}
		}
		return nil, nil
	}
	if err := rejectInlineSecrets(m); err != nil {
		return nil, err
	}
	c := m.Credentials
	creds := &nango.Credentials{
		Type:          strings.TrimSpace(c.Type),
		Scopes:        strings.TrimSpace(c.Scopes),
		AppLink:       strings.TrimSpace(c.AppLink),
		ClientName:    strings.TrimSpace(c.ClientName),
		ClientUri:     strings.TrimSpace(c.ClientURI),
		ClientLogoUri: strings.TrimSpace(c.ClientLogoURI),
	}
	if creds.Type == "" {
		creds.Type = provider.AuthMode
	}
	var missing []string
	assignEnv(&creds.ClientID, c.ClientIDEnv, &missing)
	assignEnv(&creds.ClientSecret, c.ClientSecretEnv, &missing)
	assignEnv(&creds.AppID, c.AppIDEnv, &missing)
	assignEnv(&creds.AppLink, c.AppLinkEnv, &missing)
	assignEnv(&creds.PrivateKey, c.PrivateKeyEnv, &missing)
	assignEnv(&creds.WebhookSecret, c.WebhookSecretEnv, &missing)
	assignEnv(&creds.Username, c.UsernameEnv, &missing)
	assignEnv(&creds.Password, c.PasswordEnv, &missing)
	if len(missing) > 0 {
		if m.Required {
			return nil, fmt.Errorf("%s requires env var(s): %s", m.ID, strings.Join(missing, ", "))
		}
		return nil, skippedIntegration{reason: "missing optional env var(s): " + strings.Join(missing, ", ")}
	}
	if err := validateCredentials(provider, creds); err != nil {
		return nil, err
	}
	return creds, nil
}

func assignEnv(dst *string, envName string, missing *[]string) {
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return
	}
	value := os.Getenv(envName)
	if value == "" {
		*missing = append(*missing, envName)
		return
	}
	*dst = value
}

func credentialsRequired(mode string) bool {
	switch mode {
	case "OAUTH1", "OAUTH2", "TBA", "APP", "CUSTOM", "MCP_OAUTH2", "MCP_OAUTH2_GENERIC", "INSTALL_PLUGIN":
		return true
	default:
		return false
	}
}

func validateCredentials(provider nango.Provider, creds *nango.Credentials) error {
	if creds == nil {
		return nil
	}
	mode := provider.AuthMode
	switch mode {
	case "OAUTH1", "OAUTH2", "TBA":
		if creds.Type != mode {
			return fmt.Errorf("credentials.type must be %q for provider %q", mode, provider.Name)
		}
		if creds.ClientID == "" || creds.ClientSecret == "" {
			return fmt.Errorf("client_id and client_secret are required for %s auth mode", mode)
		}
	case "APP":
		if creds.Type != "APP" {
			return fmt.Errorf("credentials.type must be \"APP\" for provider %q", provider.Name)
		}
		if creds.AppID == "" || creds.PrivateKey == "" {
			return fmt.Errorf("app_id and private_key are required for APP auth mode")
		}
		if provider.Name == "github-app" && creds.AppLink == "" {
			return fmt.Errorf("app_link is required for provider %q", provider.Name)
		}
	case "CUSTOM":
		if provider.Name == "github-app-oauth" {
			if creds.Type != "APP" {
				return fmt.Errorf("credentials.type must be \"APP\" for provider %q", provider.Name)
			}
			if creds.AppID == "" || creds.AppLink == "" || creds.PrivateKey == "" {
				return fmt.Errorf("app_id, app_link, and private_key are required for provider %q", provider.Name)
			}
		}
	}
	return nil
}

func rejectInlineSecrets(m Manifest) error {
	rawCreds, _ := m.raw["credentials"].(map[string]interface{})
	for _, key := range []string{"client_id", "client_secret", "app_id", "private_key", "webhook_secret", "username", "password"} {
		if _, ok := rawCreds[key]; ok {
			return fmt.Errorf("%s contains inline secret field %q; use *_env", m.SourcePath, key)
		}
	}
	return nil
}
