# Filesystem Tools

Read, write, and manage files.

---

## read

Read the contents of a file.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | Yes | Absolute path to the file |
| `offset` | number | No | Line number to start reading from (1-indexed) |
| `limit` | number | No | Maximum lines to read (default: 2000) |

### Limits

- **Maximum file size**: 50KB (files larger than this are truncated)
- **Maximum line length**: 2000 characters (longer lines are truncated with `...`)
- **Default line limit**: 2000 lines per request
- **Line numbering**: Output includes line numbers (e.g., `1: content`)

### Encoding & File Types

- **Text files**: UTF-8 encoding only
- **Binary files**: Rejected with an error (use `bash` tool for binary inspection)
- **Image files** (png, jpg, jpeg, gif, webp, bmp, ico): Returned as base64-encoded JSON
- **PDF files**: Returned as base64-encoded JSON
- **SVG files**: Treated as text (not images)

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
  "content": "1: # Project Name\n2: \n3: This is a project.\n",
  "total_lines": 3,
  "lines_read": 3,
  "truncated": false
}
```

### Reading Partial Files

Read lines 10-20 (offset is 1-indexed):

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

### Reading Directories

When `path` is a directory, returns a paginated list of entries:

```json
{
  "name": "read",
  "arguments": {
    "path": "/home/user/project/src"
  }
}
```

### Common Errors

| Error | Cause |
|-------|-------|
| `File not found` | Path doesn't exist (suggests similar filenames if available) |
| `Permission denied` | Can't read file |
| `Is a directory` | Note: Directories are actually listed, not rejected |
| `Binary file detected` | Binary file detected by content analysis or extension |
| `file_path must be an absolute path` | Relative paths are not allowed |

---

## write

Create a new file or overwrite an existing file.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | Yes | Absolute path to create/overwrite |
| `content` | string | Yes | File contents |

### Behavior

- **Atomicity**: NOT atomic — writes directly to the target file
- **Parent directories**: Created automatically if they don't exist
- **Staleness check**: For existing files, the file must have been read first to prevent overwriting unseen changes
- **File locking**: Concurrent writes to the same file are serialized

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
  "path": "/home/user/project/config.json",
  "bytes_written": 23,
  "created": true,
  "diff": null,
  "diagnostics": null
}
```

### Common Errors

| Error | Cause |
|-------|-------|
| `filePath must be an absolute path` | Relative paths are not allowed |
| `File has been modified` | Staleness check failed — file changed since last read |
| `Permission denied` | Can't write to location |

---

## edit

Make targeted changes to a file using fuzzy matching.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | Yes | Absolute path to edit |
| `old_string` | string | Yes | Text to find and replace |
| `new_string` | string | Yes | Replacement text (must differ from old_string) |
| `replace_all` | boolean | No | If true, replace all occurrences (default: false) |

### Matching Behavior

The edit tool uses a chain of **9 matching strategies** (first match wins):

1. **Simple** — Exact string match
2. **LineTrimmed** — Trim each line before matching
3. **BlockAnchor** — Levenshtein-based fuzzy block matching (60% similarity threshold)
4. **WhitespaceNormalized** — Collapse all whitespace to single spaces
5. **IndentationFlexible** — Strip leading whitespace, match, then reindent
6. **EscapeNormalized** — Normalize escape sequences (`\n`, `\t`, etc.)
7. **TrimmedBoundary** — Trim first/last lines of old_string and match inner content
8. **ContextAware** — Use surrounding context lines to locate block
9. **MultiOccurrence** — When multiple matches exist, picks the first occurrence

### Line Ending Normalization

CRLF (`\r\n`) and CR (`\r`) line endings are automatically normalized to LF (`\n`) before matching.

### Special Case: Empty old_string

When `old_string` is empty:
- If file doesn't exist: Creates a new file with `new_string` content
- If file exists: Appends `new_string` to the end of the file

### Replace All Mode

Set `replace_all: true` to replace all occurrences of `old_string`. By default, only the first match is replaced (unless multiple matches prevent unique identification).

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
  "path": "/home/user/project/config.json",
  "old_content_snippet": "\"version\": \"1.0.0\"",
  "new_content_snippet": "\"version\": \"1.1.0\"",
  "replacements_made": 1,
  "lines_added": 0,
  "lines_removed": 0,
  "diff": "...",
  "diagnostics": null
}
```

### Common Errors

| Error | Cause |
|-------|-------|
| `oldString and newString are identical` | No change requested |
| `oldString not found in file content` | Text could not be matched by any strategy |
| `Found multiple matches for oldString` | Multiple matches and no `replace_all` flag |
| `File has been modified` | Staleness check failed — file changed since last read |

---

## ls

List directory contents with tree-like formatting.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | Yes | Absolute path to directory |

### Limits

- **Maximum entries**: 100 (truncated if more exist)

### Default Ignored Patterns

The following directories are automatically excluded:

```
node_modules, __pycache__, .git, dist, build, target, vendor,
bin, obj, .idea, .vscode, .zig-cache, zig-out, .coverage,
coverage, tmp, temp, .cache, cache, logs, .venv, venv, env
```

Additionally, `.gitignore` patterns are respected.

### Output Format

Tree-like output with 2-space indentation:

```
src/
  main.rs
  lib.rs
Cargo.toml
```

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
  "output": "src/\n  main.rs\nREADME.md\n",
  "total_entries": 3,
  "truncated": false
}
```

### Common Errors

| Error | Cause |
|-------|-------|
| `Path does not exist` | Directory doesn't exist |
| `Not a directory` | Path is a file, not a directory |

---

## glob

Find files matching a glob pattern.

### Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pattern` | string | Yes | Glob pattern (e.g., `**/*.js`) |
| `path` | string | No | Starting directory (default: current working directory) |

### Limits

- **Maximum results**: 1000 files (truncated if more match)

### Pattern Syntax

Uses standard glob pattern matching:

| Pattern | Matches |
|---------|---------|
| `*` | Any characters except `/` |
| `**` | Any characters including `/` (recursive) |
| `?` | Any single character |
| `{a,b}` | Either `a` or `b` |
| `[abc]` | Any character in the set |

### Sort Order

Results are sorted by modification time (newest first).

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
  "files": [
    {
      "path": "/home/user/project/src/components/Button.test.js",
      "modified": "2024-01-15T10:30:00Z"
    }
  ],
  "total_matches": 1,
  "truncated": false
}
```

### Common Errors

| Error | Cause |
|-------|-------|
| `Invalid glob pattern` | Pattern syntax error |
| `Path does not exist` | Search directory doesn't exist |

---

## Security

**Note**: Path sandboxing is currently **disabled**. Filesystem tools can access:

- Any path on the host filesystem
- No restrictions on parent directory access (`../`)

Configure the working directory via:

- MCP filesystem server
- Tool configuration

---

## See Also

- [bash](bash-tool.md) — For complex file operations and binary file inspection
- [grep](search-tools.md) — Search file contents
