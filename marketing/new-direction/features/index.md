# ZiraLoop Feature Pages — "The Secure Access Layer for Your AI Agents"

---

## Site Navigation

```
Products
  ├── Auth          /products/auth
  ├── Vault         /products/vault
  ├── Permissions   /products/permissions
  ├── MCP           /products/mcp
  ├── Proxy         /products/proxy
  ├── BYOK          /products/byok
  └── Observability /products/observability
```

---

## Feature Pages

| Page | File | Hero Line | Primary Problem |
|---|---|---|---|
| [Auth](auth.md) | `auth.md` | One connect flow. Every credential. | Fragmented Connections |
| [Vault](vault.md) | `vault.md` | Enterprise-grade credential custody. | Scattered Secrets |
| [Permissions](permissions.md) | `permissions.md` | Zero-trust permissions for every agent. | God-Mode Credentials |
| [MCP](mcp.md) | `mcp.md` | A scoped MCP server for every agent. | God-Mode Credentials |
| [Proxy](proxy.md) | `proxy.md` | Sub-5ms proxy to any provider. | Scattered Secrets |
| [BYOK](byok.md) | `byok.md` | Ship Bring Your Own Key in days. | All three |
| [Observability](observability.md) | `observability.md` | See everything your agents do. | Scattered Secrets |

---

## Problem-to-Feature Mapping

| Feature | Problem 1: God-Mode Credentials | Problem 2: Scattered Secrets | Problem 3: Fragmented Connections |
|---|:---:|:---:|:---:|
| **Auth** | | | Primary |
| **Vault** | | Primary | |
| **Permissions** | Primary | | |
| **MCP** | Primary | | Secondary |
| **Proxy** | | Primary | Secondary |
| **BYOK** | Secondary | Primary | Primary |
| **Observability** | Secondary | Primary | Secondary |
