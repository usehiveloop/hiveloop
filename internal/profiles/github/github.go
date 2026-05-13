package githubprofile

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const Provider = "github"

func EncryptIdentity(encKey *crypto.SymmetricKey, identity model.JSON) ([]byte, error) {
	if encKey == nil {
		return nil, fmt.Errorf("github profile identity encryption is not configured")
	}
	plaintext, err := json.Marshal(identity)
	if err != nil {
		return nil, fmt.Errorf("encode github profile identity: %w", err)
	}
	defer zeroBytes(plaintext)
	encrypted, err := encKey.Encrypt(plaintext)
	if err != nil {
		return nil, fmt.Errorf("encrypt github profile identity: %w", err)
	}
	return encrypted, nil
}

func DecryptIdentity(encKey *crypto.SymmetricKey, profile model.AgentProfile) (model.JSON, error) {
	if len(profile.EncryptedIdentity) == 0 {
		return cloneJSON(profile.Identity), nil
	}
	if encKey == nil {
		return nil, fmt.Errorf("github profile identity encryption is not configured")
	}
	plaintext, err := encKey.Decrypt(profile.EncryptedIdentity)
	if err != nil {
		return nil, fmt.Errorf("decrypt github profile identity: %w", err)
	}
	defer zeroBytes(plaintext)
	identity := model.JSON{}
	if err := json.Unmarshal(plaintext, &identity); err != nil {
		return nil, fmt.Errorf("decode github profile identity: %w", err)
	}
	return identity, nil
}

func GitAuthor(identity model.JSON, fallbackName string) (string, string) {
	name := FirstString(identity, "name", "login")
	if name == "" {
		name = fallbackName
	}
	email := strings.TrimSpace(FirstString(identity, "email"))
	if email == "" {
		email = NoreplyEmail(identity)
	}
	return name, email
}

func FirstString(identity model.JSON, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringFromAny(identity[key])); value != "" {
			return value
		}
	}
	return ""
}

func NoreplyEmail(identity model.JSON) string {
	login := strings.TrimSpace(FirstString(identity, "login"))
	if login == "" {
		return ""
	}
	id := strings.TrimSpace(stringFromAny(identity["id"]))
	if id == "" {
		return login + "@users.noreply.github.com"
	}
	return id + "+" + login + "@users.noreply.github.com"
}

func cloneJSON(in model.JSON) model.JSON {
	out := model.JSON{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringFromAny(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case float64:
		return fmt.Sprintf("%.0f", value)
	case int:
		return fmt.Sprint(value)
	case int64:
		return fmt.Sprint(value)
	case json.Number:
		return value.String()
	case fmt.Stringer:
		return value.String()
	default:
		return ""
	}
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
