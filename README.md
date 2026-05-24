# Hivy

Please read AGENTS.md file before starting.

## Local Development

After copying `.env.example` to `.env` and filling required secrets, run:

```bash
make dev
```

`make dev` uses Docker Compose as the source of truth, but runs development
processes: the Go API and worker restart with Air when backend files change,
and `apps/web` runs `next dev` with the app directory mounted into the
container.

The default Postgres host port for `make dev` is `15432` so it does not collide
with the test stack on `5433`. Compose-only knobs use the `HIVY_COMPOSE_*`
prefix; override values like `HIVY_COMPOSE_POSTGRES_PORT` in `.env` if you need
different host ports.
Redis binds to `16379` on the host for the same reason; inside Compose, Hivy
still uses `redis:6379`.

## Local Env Secrets

Start from the checked-in template:

```bash
cp .env.example .env
```

Generate the common local secrets with:

```bash
{
  echo "HIVY_SESSION_SECRET=$(openssl rand -base64 32)"
  echo "HIVY_JWT_SIGNING_KEY=$(openssl rand -base64 32)"
  echo "HIVY_AUTH_RSA_PRIVATE_KEY=$(openssl genrsa 2048 | base64 | tr -d '\n')"
  echo "HIVY_KMS_TYPE=aead"
  echo "HIVY_KMS_KEY=$(openssl rand -base64 32)"
  echo "HIVY_SANDBOX_ENCRYPTION_KEY=$(openssl rand -base64 32)"
  echo "HIVY_NANGO_ENCRYPTION_KEY=$(openssl rand -base64 32)"
}
```

Paste those values into `.env`. Provider credentials and LLM keys still need to be real values for flows that call external services.
