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
2. Bridge connects to the server on startup
3. The server advertises its tools
4. Bridge exposes those tools to the agent
5. When the agent uses a tool, Bridge forwards the call to the server

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

#### HTTP
Connects to a remote server over HTTP:

```json
{
  "type": "http",
  "url": "https://mcp.example.com/tools",
  "headers": {
    "Authorization": "Bearer <bridge-control-plane-api-key>"
  }
}
```

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

## Tool Namespacing

MCP tools are namespaced by server name:

```
filesystem__read_file    # From "filesystem" server
warehouse__run_query     # From "warehouse" server
```

The agent sees these names and can call them like any other tool.

---

## Error Handling

If an MCP server crashes or becomes unavailable:

- Active tool calls fail with an error
- Bridge logs the disconnection
- The agent receives the error and can decide what to tell the user

Restart Bridge to reconnect to MCP servers.

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

Look for lines like:

```
DEBUG mcp::connection > Sending request to filesystem: {...}
DEBUG mcp::connection > Received response: {...}
```

---

## See Also

- [Tools](tools.md) — Built-in tools overview
- [Custom Tools](../tools-reference/custom-tools.md) — Other ways to extend Bridge
- [MCP Specification](https://modelcontextprotocol.io) — Full protocol spec
