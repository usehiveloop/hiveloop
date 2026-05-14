# Custom Tools

Build your own tools for Bridge.

---

## Overview

You can extend Bridge with custom tools in two ways:

1. **Integration tools** — HTTP endpoints Bridge calls
2. **MCP servers** — External tool servers

---

## Integration Tools

Define HTTP endpoints that Bridge calls as tools.

### Configuration

Add to your agent definition:

```json
{
  "integrations": [
    {
      "name": "crm",
      "description": "Customer relationship management",
      "base_url": "https://api.your-crm.com",
      "headers": {
        "Authorization": "Bearer <bearer-token>"
      },
      "actions": [
        {
          "name": "get_customer",
          "description": "Get customer by ID",
          "method": "GET",
          "path": "/customers/{customer_id}",
          "parameters_schema": {
            "type": "object",
            "properties": {
              "customer_id": { "type": "string" }
            },
            "required": ["customer_id"]
          }
        },
        {
          "name": "create_ticket",
          "description": "Create a support ticket",
          "method": "POST",
          "path": "/tickets",
          "parameters_schema": {
            "type": "object",
            "properties": {
              "title": { "type": "string" },
              "description": { "type": "string" }
            },
            "required": ["title", "description"]
          },
          "permission": "require_approval"
        }
      ]
    }
  ]
}
```

### How It Works

When the agent uses `crm__get_customer`:

1. Bridge makes HTTP request to your endpoint
2. Your server processes and responds
3. Bridge returns the result to the agent

### Request Format

Bridge sends:

```
GET /customers/123 HTTP/1.1
Host: api.your-crm.com
Authorization: Bearer <bearer-token>
```

### Response Format

Your server should return:

```json
{
  "success": true,
  "result": { "id": "123", "name": "John Doe" }
}
```

Or on error:

```json
{
  "success": false,
  "error": "Customer not found"
}
```

### Permissions

Per-action permission settings:

```json
{
  "name": "delete_customer",
  "permission": "require_approval"
}
```

---

## MCP Servers

Build custom tools using the Model Context Protocol.

### Simple MCP Server (Node.js)

```javascript
const { Server } = require('@modelcontextprotocol/sdk/server');
const { StdioServerTransport } = require('@modelcontextprotocol/sdk/server/stdio');

const server = new Server({
  name: 'my-custom-server',
  version: '1.0.0'
});

// List available tools
server.setRequestHandler('tools/list', async () => {
  return {
    tools: [
      {
        name: 'calculate_price',
        description: 'Calculate order total with tax',
        parameters: {
          type: 'object',
          properties: {
            items: {
              type: 'array',
              items: { type: 'object' }
            },
            tax_rate: { type: 'number' }
          },
          required: ['items']
        }
      }
    ]
  };
});

// Handle tool calls
server.setRequestHandler('tools/call', async (request) => {
  if (request.params.name === 'calculate_price') {
    const { items, tax_rate = 0.08 } = request.params.arguments;
    const subtotal = items.reduce((sum, item) => sum + item.price, 0);
    const tax = subtotal * tax_rate;
    const total = subtotal + tax;
    
    return {
      content: [{
        type: 'text',
        text: JSON.stringify({ subtotal, tax, total })
      }]
    };
  }
});

server.connect(new StdioServerTransport());
```

### Configure in Agent

```json
{
  "mcp_servers": [
    {
      "name": "pricing",
      "transport": {
        "type": "stdio",
        "command": "node",
        "args": ["/path/to/pricing-server.js"]
      }
    }
  ]
}
```

### Tool Namespacing

MCP tools are namespaced by server name:

```
pricing__calculate_price    # From "pricing" server
```

---

## Choosing Integration vs MCP

| | Integration Tools | MCP Servers |
|---|-------------------|-------------|
| **Hosting** | Your existing API | Separate process |
| **Language** | Any (HTTP) | Any (MCP SDK available) |
| **Complexity** | Simple HTTP | Full protocol |
| **State** | Stateless | Can maintain state |
| **Best for** | Existing APIs, simple actions | Complex logic, multiple tools |

---

## Best Practices

### Keep Tools Focused

Each tool should do one thing well.

**Good:** `get_customer`, `update_customer`, `create_ticket`

**Too broad:** `do_crm_stuff`

### Write Clear Descriptions

The AI uses descriptions to decide which tool to use:

```json
{
  "name": "get_customer",
  "description": "Look up a customer by their ID. Use this when the user mentions a customer ID or asks about a specific customer."
}
```

### Validate Input

Check parameters and return clear errors:

```json
{
  "success": false,
  "error": "customer_id is required"
}
```

### Set Appropriate Permissions

Use `require_approval` for destructive operations:

```json
{
  "name": "delete_customer",
  "description": "Permanently delete a customer",
  "permission": "require_approval"
}
```

---

## See Also

- [MCP](../core-concepts/mcp.md) — MCP overview
- [Integration Guide](../control-plane/index.md) — Building your control plane
