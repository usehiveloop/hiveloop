# MCP (Model Context Protocol)

MCP lets Bridge connect to external tool servers. It's a standard way to add capabilities like filesystem access, database queries, or any custom functionality.

---

## What is MCP?

Model Context Protocol (MCP) is an open protocol for connecting AI systems to external tools and data sources. Think of it like USB for AI capabilities:

- A standard way to describe what tools are available
- A standard way to call those tools
- Works with any MCP-compatible server

```
Bridge ◄──MCP──► Filesystem Server
       ◄──MCP──► Database Server
       ◄──MCP──► Custom Server (your code)
```

---

## How It Works

1. You configure an MCP server in your agent definition (or per-conversation — see below)
2. Bridge connects to the server on agent startup (or at conversation creation for per-conv servers)
3. The server advertises its tools
4. Bridge registers those tools alongside built-in tools
5. When the agent uses a tool, Bridge forwards the call to the MCP server
6. MCP server responses are extracted and returned to the agent

Bridge supports MCP servers at two scopes:

| Scope | Configured at | Lifetime | Use when |
|---|---|---|---|
| **Agent-level** | Agent definition `mcp_servers` | Until the agent is removed or updated | The tool surface is the same for every conversation |
| **Per-conversation** | `POST /agents/{id}/conversations` body `mcp_servers` | Only that conversation's lifetime | Tools vary per call (e.g. tenant tokens, dev-only sessions) |

Per-conversation MCP is described in [Per-Conversation MCP Servers](#per-conversation-mcp-servers) below.

---

## Configuring MCP Servers

Add MCP servers to your agent:

```json
{
  "id": "my-agent",
  "mcp_servers": [
    {
      "name": "filesystem",
      "transport": {
        "type": "stdio",
        "command": "npx",
        "args": ["@modelcontextprotocol/server-filesystem", "/home/user/project"],
        "env": {}
      }
    },
    {
      "name": "postgres",
      "transport": {
        "type": "stdio",
        "command": "node",
        "args": ["/path/to/postgres-server.js"],
        "env": {
          "DATABASE_URL": "postgres://localhost/mydb"
        }
      }
    }
  ]
}
```

### Transport Types

Bridge supports two transport types:

#### stdio
Runs a local command and communicates over stdin/stdout:

```json
{
  "type": "stdio",
  "command": "npx",
  "args": ["@modelcontextprotocol/server-filesystem", "/workspace"],
  "env": {
    "NODE_ENV": "production"
  }
}
```

**Fields:**
- `command` (required) — The executable to run
- `args` (optional) — Array of command-line arguments
- `env` (optional) — Environment variables to set for the process

#### streamable_http
Connects to a remote server over HTTP using the MCP streamable HTTP transport:

```json
{
  "type": "streamable_http",
  "url": "https://mcp.example.com/tools",
  "headers": {
    "Authorization": "Bearer <bearer-token>"
  }
}
```

**Fields:**
- `url` (required) — The MCP server endpoint URL
- `headers` (optional) — Additional HTTP headers to include in requests

**Transport Differences:**

| Feature | stdio | streamable_http |
|---------|-------|-----------------|
| Use case | Local tools | Remote services |
| Process lifecycle | Bridge spawns & manages | External server manages |
| Network required | No | Yes |
| Latency | Lower | Higher (network) |

---

## Available MCP Servers

The MCP community provides servers for common tasks:

| Server | What it does |
|--------|--------------|
| `@modelcontextprotocol/server-filesystem` | Read/write files |
| `@modelcontextprotocol/server-postgres` | Query PostgreSQL |
| `@modelcontextprotocol/server-sqlite` | Query SQLite |
| `@modelcontextprotocol/server-puppeteer` | Browser automation |

Find more at [github.com/modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers).

---

## MCP vs Built-in Tools

When should you use MCP vs Bridge's built-in tools?

| Use MCP when... | Use built-in when... |
|-----------------|----------------------|
| You need custom logic | Standard filesystem operations |
| Accessing external databases | Simple bash commands |
| Using community tools | Web search/fetch |
| Tool needs special environment | General purpose tasks |

You can use both together:

```json
{
  "id": "data-analyst",
  "tools": ["read", "write", "bash"],
  "mcp_servers": [{
    "name": "warehouse",
    "transport": {
      "type": "stdio",
      "command": "node",
      "args": ["./bigquery-server.js"]
    }
  }]
}
```

---

## Tool Discovery

When an agent loads, Bridge connects to all configured MCP servers and discovers their available tools. This happens:

- **Once per agent** — When the agent is first loaded or updated
- **Before conversations start** — Tools must be discovered before any conversation begins
- **Independently per agent** — Each agent has its own MCP connections

If an MCP server fails to connect or list tools, Bridge logs the error and continues loading the agent without that server's tools. Other MCP servers and built-in tools remain available.

Per-conversation MCP servers are discovered *synchronously* at conversation creation time — if any step fails, the create request returns HTTP 400 and the conversation is never created.

---

## Per-Conversation MCP Servers

Since **v0.18.0**, you can attach MCP servers to a single conversation instead of — or in addition to — the agent's own MCP definitions. This is useful when the tool surface is request-specific: a tenant-scoped HTTP MCP server with per-user credentials, a dev-only tool that shouldn't be visible in production agents, or a short-lived integration you don't want to bake into the agent definition.

### Request

Send `mcp_servers` in the `POST /agents/{agent_id}/conversations` body:

```bash
curl -X POST http://localhost:8080/agents/my-agent/conversations \
  -H "Content-Type: application/json" \
  -d '{
    "mcp_servers": [
      {
        "name": "tenant_data",
        "transport": {
          "type": "streamable_http",
          "url": "https://mcp.example.com/v1",
          "headers": {
            "Authorization": "Bearer <tenant-token>",
            "X-Tenant-Id": "42"
          }
        }
      }
    ]
  }'
```

Each entry has the same `McpServerDefinition` shape used in agent definitions:

```json
{
  "name": "<unique-server-name>",
  "transport": { "type": "streamable_http", "url": "...", "headers": { ... } }
}
```

Multiple servers are supported. See the [Conversations API](../api-reference/conversations-api.md#per-conversation-mcp) reference for the full field list.

### How the tools are merged

When a conversation is created with `mcp_servers`, Bridge:

1. Connects to each server under a scope keyed by the conversation's UUID
2. Calls `tools/list` on each and bridges the results into `ToolExecutor`s
3. **Merges** those executors into the conversation's tool set, **on top of** whatever the agent's own tools provided (after any `tool_names` / `mcp_server_names` filters)
4. Builds a conversation-scoped rig-core agent so this conversation sees the extended tool set — other conversations sharing the same agent are unaffected
5. Spawns the conversation loop

If step 1 or 2 fails for any server, Bridge disconnects anything that connected partially and returns an HTTP 400 with the specific error. No leaked processes, no dangling handles.

### Lifecycle and cleanup

Per-conv MCP connections are torn down on **every** conversation termination path:

- `DELETE /conversations/{id}` — normal end
- `POST /conversations/{id}/abort` — turn abort (conversation stays alive → connection stays up)
- `SIGINT` / `SIGTERM` — process shutdown
- Agent drain / update via control plane — the drain cancels the conversation, which runs cleanup
- `max_turns` reached
- Internal error that breaks the loop

The one edge case: a hard process panic or `kill -9` skips the cleanup block. For everything else, disconnect is guaranteed.

Note: **abort** cancels only the current turn, not the conversation. Per-conv MCP connections remain attached across aborts so the next message in the same conversation sees the same tool set.

### Name collisions

If a per-conversation MCP server advertises a tool whose name matches one the agent already has (built-in like `Glob`, `Grep`, `Read`, or any agent-level MCP tool), the create request is **rejected with HTTP 400**. No shadowing, no auto-prefixing.

Two ways to resolve:

1. **Rename the tool on the MCP side** (preferred — tool names should be unique)
2. **Filter the colliding built-in out of the base surface for this conversation** using `tool_names`:

```json
{
  "tool_names": ["bash"],
  "mcp_servers": [
    {
      "name": "project_fs",
      "transport": { "type": "streamable_http", "url": "https://fs.example.com/mcp" }
    }
  ]
}
```

The filter drops `Glob`/`Grep`/`Read`/etc. from the agent's base, and the MCP server's own versions take their place. The filter applies before the per-conv MCP merge, so this always works.

### Security: `allow_stdio_mcp_from_api`

`streamable_http` per-conv servers are safe(-ish): they make outbound network calls from Bridge to an HTTP endpoint you specify. `stdio` per-conv servers spawn a subprocess with Bridge's privileges and inherit its environment — this is a foot-gun in multi-tenant or API-exposed deployments.

Bridge gates stdio transport from the API behind an opt-in runtime flag, **off by default**:

```bash
# Env var
export BRIDGE_ALLOW_STDIO_MCP_FROM_API=true

# Or in config.toml
allow_stdio_mcp_from_api = true
```

When the flag is `false` (default), sending a stdio-transport server in `mcp_servers` returns:

```
HTTP 400 invalid_request
mcp_servers: stdio transport not allowed from API (server '<name>');
enable allow_stdio_mcp_from_api in runtime config to permit it
```

Agent-level MCP servers are **not** affected by this flag — they come from trusted agent definitions pushed by the control plane, not from API callers. The gate is strictly on caller-supplied stdio servers.

**Rule of thumb:** leave the flag off unless every caller that can reach the API is fully trusted AND Bridge is already sandboxed (container with restricted filesystem/network).

### Validation errors

All validation runs before any network or process work, so a rejected request never spawns anything. Each returns HTTP 400 with `code: "invalid_request"`:

| Error | Cause |
|---|---|
| `mcp_servers: server name cannot be empty` | Empty or whitespace-only `name` |
| `mcp_servers: duplicate server name '<name>'` | Two entries with the same `name` in one request |
| `mcp_servers: stdio transport not allowed from API` | Stdio used with `allow_stdio_mcp_from_api=false` |
| `mcp_servers: failed to connect to '<name>'` | MCP handshake failed |
| `mcp_servers: failed to list tools from '<name>': ...` | Connect OK, `tools/list` failed |
| `mcp_servers: tool '<tool>' from server '<name>' collides with an existing agent tool` | Name collision (see above) |

### When to use which scope

| Use agent-level MCP when... | Use per-conversation MCP when... |
|---|---|
| Every conversation needs the same tools | Tool surface varies by caller, tenant, or session |
| The server's URL / credentials are static | Credentials are request-specific (per-user tokens, tenant IDs in headers) |
| You want zero latency on conversation start | You can absorb one MCP handshake per conversation |
| You control the agent definition | Callers without control-plane access need to extend the tool surface |

Per-conv MCP pays a connection cost on every `POST /conversations` — prefer agent-level when the tools are constant.

---

## Connection Lifecycle

```
Agent Load
    │
    ▼
Connect to MCP servers ──► Connection failed ──► Log error, continue
    │
    ▼
List tools from each server
    │
    ▼
Register tools in ToolRegistry
    │
    ▼
Agent ready for conversations
    │
    ▼
Tool call ──► Forward to MCP server ──► Return result
    │
    ▼
Agent removed / updated
    │
    ▼
Disconnect all MCP servers
```

**Key behaviors:**
- Connections are established at agent load time, not per conversation
- All agent conversations share the same MCP connections
- Connections are closed when the agent is removed or updated
- There is no automatic reconnection if an MCP server crashes during operation

---

## Error Handling

### Connection Failures

If an MCP server fails to connect during agent startup:
- The error is logged
- The agent continues to load without that server's tools
- Other MCP servers and built-in tools work normally

### Tool Call Failures

If an MCP server crashes or becomes unavailable during a tool call:
- The tool call fails with an error message
- The error is returned to the agent
- The agent can decide how to handle the failure

**Example error message:**
```
mcp error: failed to call tool 'read_file' on 'filesystem': <underlying error>
```

### Recovery

To reconnect to MCP servers after a failure:
- **Agent update** — The control plane pushes an agent update, which triggers reconnection
- **Bridge restart** — Restarting Bridge reloads all agents and reconnects to MCP servers

---

## Tool Names

MCP tools are registered with their original names as provided by the MCP server. Bridge does not add prefixes or namespaces to tool names.

**Example:**
- An MCP server provides a tool named `read_file`
- The agent sees and calls it as `read_file`

**Avoiding Name Conflicts:**
If an MCP tool has the same name as a built-in tool:
- The MCP tool registration may conflict
- It is recommended to configure MCP servers with unique tool names

---

## Limits

| Resource | Limit | Notes |
|----------|-------|-------|
| MCP servers per agent | No hard limit | Limited by system resources |
| Concurrent connections | Per-agent | Each agent has isolated connections |
| Tool count | No hard limit | Limited by LLM context window |

---

## Building Your Own MCP Server

You can build custom MCP servers for your specific needs:

```javascript
// my-server.js
const { Server } = require('@modelcontextprotocol/sdk');

const server = new Server({
  name: 'my-custom-server',
  version: '1.0.0'
});

server.setRequestHandler('tools/list', async () => {
  return {
    tools: [{
      name: 'lookup_customer',
      description: 'Look up customer by ID',
      parameters: {
        type: 'object',
        properties: {
          customer_id: { type: 'string' }
        },
        required: ['customer_id']
      }
    }]
  };
});

server.setRequestHandler('tools/call', async (request) => {
  if (request.params.name === 'lookup_customer') {
    const customer = await db.findCustomer(request.params.arguments.customer_id);
    return {
      content: [{ type: 'text', text: JSON.stringify(customer) }]
    };
  }
});

server.connect(new StdioServerTransport());
```

See the [MCP SDK documentation](https://github.com/modelcontextprotocol) for details.

---

## Security Considerations

MCP servers run with the same permissions as Bridge:

- **stdio servers** — Can access the filesystem, network, etc.
- **Only run trusted code** — MCP servers have full system access
- **Use environment variables** — For secrets, not command-line args
- **Consider sandboxing** — Run MCP servers in containers for isolation

---

## Debugging MCP

Enable debug logging to see MCP communication:

```bash
export BRIDGE_LOG_LEVEL=debug
./bridge
```

Look for log lines like:

```
INFO mcp::manager > connected to MCP server agent_id="my-agent" server="filesystem"
INFO mcp::manager > discovered MCP tools agent_id="my-agent" server="filesystem" tool_count=5
ERROR mcp::manager > failed to connect to MCP server agent_id="my-agent" server="bad-server" error="..."
```

---

## See Also

- [Tools](tools.md) — Built-in tools overview
- [Custom Tools](../tools-reference/custom-tools.md) — Other ways to extend Bridge
- [MCP Specification](https://modelcontextprotocol.io) — Full protocol spec
