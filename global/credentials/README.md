# Global LLM Credentials

`llm.json` is a non-secret startup manifest for platform-owned LLM API keys.
It must contain environment variable names only. Do not commit actual API keys.

At startup, the backend reads each enabled entry, loads the plaintext key from
`api_key_env`, encrypts it with the configured KMS wrapper, and upserts a system
credential owned by the platform org.

Every entry must include an explicit `base_url`; the seeder never derives URLs
from the model registry. `provider_id` must be one of the supported seed
providers in `internal/credentials/global_llm_seed.go`.

Optional entries with missing environment variables are skipped. Set
`required: true` only when the app must fail startup if that key is absent.
