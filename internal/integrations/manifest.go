package integrations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

type Manifest struct {
	Version           int                    `json:"version"`
	ID                string                 `json:"id"`
	Provider          string                 `json:"provider"`
	NangoProvider     string                 `json:"nango_provider,omitempty"`
	UniqueKey         string                 `json:"unique_key"`
	DisplayName       string                 `json:"display_name"`
	Enabled           *bool                  `json:"enabled,omitempty"`
	Required          bool                   `json:"required,omitempty"`
	SupportsRAGSource bool                   `json:"supports_rag_source,omitempty"`
	Meta              model.JSON             `json:"meta,omitempty"`
	Credentials       *CredentialsManifest   `json:"credentials,omitempty"`
	AllowNoCatalog    bool                   `json:"allow_no_catalog,omitempty"`
	SourcePath        string                 `json:"-"`
	raw               map[string]interface{} `json:"-"`
}

type CredentialsManifest struct {
	Type             string `json:"type"`
	ClientIDEnv      string `json:"client_id_env,omitempty"`
	ClientSecretEnv  string `json:"client_secret_env,omitempty"`
	Scopes           string `json:"scopes,omitempty"`
	AppIDEnv         string `json:"app_id_env,omitempty"`
	AppLinkEnv       string `json:"app_link_env,omitempty"`
	AppLink          string `json:"app_link,omitempty"`
	PrivateKeyEnv    string `json:"private_key_env,omitempty"`
	WebhookSecretEnv string `json:"webhook_secret_env,omitempty"`
	ClientName       string `json:"client_name,omitempty"`
	ClientURI        string `json:"client_uri,omitempty"`
	ClientLogoURI    string `json:"client_logo_uri,omitempty"`
	UsernameEnv      string `json:"username_env,omitempty"`
	PasswordEnv      string `json:"password_env,omitempty"`
}

func loadManifests(dir string) ([]Manifest, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "global/integrations"
	}
	resolved, err := resolveDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, fmt.Errorf("read global integrations dir %q: %w", resolved, err)
	}
	out := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		manifest, err := readManifest(filepath.Join(resolved, entry.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, manifest)
	}
	return out, nil
}

func readManifest(path string) (Manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Manifest{}, fmt.Errorf("parse raw %s: %w", path, err)
	}
	manifest.SourcePath = path
	manifest.raw = raw
	return manifest, nil
}

func resolveDir(dir string) (string, error) {
	if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
		return dir, nil
	}
	if filepath.IsAbs(dir) {
		return "", os.ErrNotExist
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	for {
		candidate := filepath.Join(cwd, dir)
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return "", os.ErrNotExist
}
