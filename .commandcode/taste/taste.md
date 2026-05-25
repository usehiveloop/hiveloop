# Taste (Continuously Learned by [CommandCode][cmd])

[cmd]: https://commandcode.ai/

# Development Philosophy
- No backward compatibility required - we are still in pre-launch development. Confidence: 0.90
- Prefer clean removal/ripping out over gradual deprecation or patching. Confidence: 0.85
- All operations must be idempotent (restarting app multiple times should not break). Confidence: 0.80
- Prefer explicit configuration over implicit/automatic behavior. Confidence: 0.75

# Database & Migrations
- Use goose v3 for migrations, never auto-migrate. Confidence: 0.90
- Write migration schemas manually by studying model definitions. Confidence: 0.85
- Split large schemas into multiple readable migration files (10+ files acceptable). Confidence: 0.70

# Architecture & Naming
- Rename aggressively: in_connections -> connections, in_integrations -> integrations, etc. Confidence: 0.85
- Use consistent environment variable prefixes: HIVY_ for app, CLOUD_AGENTS_ for sandbox-related. Confidence: 0.75
- Prefer interface-based abstractions for provider-swappable components. Confidence: 0.80

# Configuration Management
- Use JSON files as source of truth for global seeds (skills, credentials, integrations, plans). Confidence: 0.85
- No admin panels - configure via JSON files synced at startup. Confidence: 0.80
- Environment variables must have clear grouping/prefixes in .env.example. Confidence: 0.75

# Testing & Verification
- Use "nuke docker compose down -v, make up, run bench" cycle for verification. Confidence: 0.85
- Manual integration testing before automation. Confidence: 0.75
- Launch parallel agents to fix test suites (3-5 files per agent). Confidence: 0.80

# Local Development
- Docker Compose is source of truth for local dev environment. Confidence: 0.80
- Support multi-platform builds (macOS and Linux). Confidence: 0.70
- Use make dev for single-command local setup. Confidence: 0.75

# Code Organization
- Prefer complete renames over maintaining aliases/compatibility. Confidence: 0.90
- Remove unused code completely rather than keeping for "future use". Confidence: 0.85
- Plans must include checkpoints for user authorization before proceeding. Confidence: 0.70

