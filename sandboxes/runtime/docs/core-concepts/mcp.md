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

1. You configure an MCP server in your agent definition
2. Bridge connects to the server on agent startup
3. The server advertises its tools
4. Bridge registers those tools alongside built-in tools
5. When the agent uses a tool, Bridge forwards the call to the MCP server
6. MCP server responses are extracted and returned to the agent

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
    "Authorization": "Bearer <bridge-control-plane-api-key>"
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
