# Filesystem Tools

Read, write, and manage files.

---

## read

Read the contents of a file.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | Yes | Path to the file |
| `offset` | number | No | Start reading from this line (0-indexed) |
| `limit` | number | No | Read at most this many lines |

### Example

```json
{
  "name": "read",
  "arguments": {
    "path": "/home/user/project/README.md"
  }
}
```

### Result

```json
{
  "success": true,
  "result": "# Project Name\n\nThis is a project.\n",
  "metadata": {
    "line_count": 3,
    "size_bytes": 35
  }
}
```

### Reading Partial Files

Read lines 10-20:

```json
{
  "name": "read",
  "arguments": {
    "path": "/home/user/project/large-file.txt",
    "offset": 10,
    "limit": 10
  }
}
```

### Common Errors

| Error | Cause |
|-------|-------|
| `File not found` | Path doesn't exist |
| `Permission denied` | Can't read file |
| `Is a directory` | Path is a directory |

---

## write

Create a new file or overwrite an existing file.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | Yes | Path to create/overwrite |
| `content` | string | Yes | File contents |

### Example

```json
{
  "name": "write",
  "arguments": {
    "path": "/home/user/project/config.json",
    "content": "{\n  \"name\": \"my-app\"\n}"
  }
}
```

### Result

```json
{
  "success": true,
  "result": "File written: /home/user/project/config.json"
}
```

### Common Errors

| Error | Cause |
|-------|-------|
| `Directory not found` | Parent directory doesn't exist |
| `Permission denied` | Can't write to location |

---

## edit

Make targeted changes to a file.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | Yes | Path to edit |
| `old_string` | string | Yes | Text to find and replace |
| `new_string` | string | Yes | Replacement text |

### Example

```json
{
  "name": "edit",
  "arguments": {
    "path": "/home/user/project/config.json",
    "old_string": "\"version\": \"1.0.0\"",
    "new_string": "\"version\": \"1.1.0\""
  }
}
```

### Result

```json
{
  "success": true,
  "result": "File edited: /home/user/project/config.json"
}
```

### When to Use edit vs write

- **edit** — Change specific lines, preserve rest of file
- **write** — Replace entire file contents

---

## ls

List directory contents.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | Yes | Directory to list |

### Example

```json
{
  "name": "ls",
  "arguments": {
    "path": "/home/user/project"
  }
}
```

### Result

```json
{
  "success": true,
  "result": [
    { "name": "src", "type": "directory" },
    { "name": "README.md", "type": "file", "size": 1024 },
    { "name": "package.json", "type": "file", "size": 512 }
  ]
}
```

---

## glob

Find files matching a pattern.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pattern` | string | Yes | Glob pattern (e.g., `**/*.js`) |
| `path` | string | No | Starting directory (default: `.`) |

### Example

```json
{
  "name": "glob",
  "arguments": {
    "pattern": "**/*.test.js",
    "path": "/home/user/project"
  }
}
```

### Result

```json
{
  "success": true,
  "result": [
    "/home/user/project/src/utils.test.js",
    "/home/user/project/src/components/Button.test.js"
  ]
}
```

### Pattern Syntax

| Pattern | Matches |
|---------|---------|
| `*.js` | All `.js` files in current directory |
| `**/*.js` | All `.js` files recursively |
| `src/**/*` | All files under `src/` |
| `*.{js,ts}` | All `.js` and `.ts` files |

---

## Security

Filesystem tools respect the working directory. Agents can only access:

- The configured working directory
- Subdirectories of that directory
- Not parent directories (`../`)

Configure the working directory via:

- MCP filesystem server
- Tool configuration

---

## See Also

- [bash](bash-tool.md) — For complex file operations
- [grep](search-tools.md) — Search file contents
