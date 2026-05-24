---
name: notion
description: Use whenever the user asks to read, search, create, edit, summarize, organize, or manage Notion pages, blocks, databases, data sources, comments, files, icons, custom emojis, templates, users, or markdown content. Provides verified curl and jq commands through the Hivy-provided Notion API proxy using NOTION_API_URL and NOTION_TOKEN, including safe patterns for Notion API version 2026-03-11 and human-facing Notion links that never use the proxy URL.
---

# Notion REST API

Use Notion through the Hivy-provided Notion API proxy at `$NOTION_API_URL`.

`NOTION_API_URL` and `NOTION_TOKEN` are provided by the Hivy runtime for the employee's configured Notion profile. Always call the provided `NOTION_API_URL` exactly; it is not expected to be `https://api.notion.com`, and the runtime handles forwarding to Notion for the selected profile connection. Do not substitute another workspace, base URL, or token.

Never use `NOTION_API_URL` to construct human-facing Notion dashboard links. It is an API proxy, not the Notion UI host. For links shown to users, use the `url` or `public_url` fields returned by Notion page, database, or data source responses. If no Notion response URL is available, provide the object ID and say the UI URL is unavailable instead of inventing one from the proxy.

## Environment

Required:

| Variable | Purpose |
|---|---|
| `NOTION_API_URL` | Hivy-provided Notion API proxy base URL |
| `NOTION_TOKEN` | Bearer token for the configured Notion connection |

Initialize once:

```bash
test -n "$NOTION_API_URL" || { echo "NOTION_API_URL is not set" >&2; exit 1; }
test -n "$NOTION_TOKEN" || { echo "NOTION_TOKEN is not set" >&2; exit 1; }
NOTION_API_URL="${NOTION_API_URL%/}"
NOTION_VERSION="${NOTION_VERSION:-2026-03-11}"
```

Use this helper for JSON API calls:

```bash
notion_api() {
  local method="$1"
  local api_path="$2"
  local body="${3:-}"
  local tmp_headers tmp_body http_status retry_after
  tmp_headers="$(mktemp)"
  tmp_body="$(mktemp)"

  for attempt in 1 2 3; do
    if test -n "$body"; then
      http_status="$(curl -sS -o "$tmp_body" -D "$tmp_headers" -w "%{http_code}" \
        -X "$method" "$NOTION_API_URL$api_path" \
        -H "Authorization: Bearer $NOTION_TOKEN" \
        -H "Notion-Version: $NOTION_VERSION" \
        -H "Content-Type: application/json" \
        --data-binary "$body")"
    else
      http_status="$(curl -sS -o "$tmp_body" -D "$tmp_headers" -w "%{http_code}" \
        -X "$method" "$NOTION_API_URL$api_path" \
        -H "Authorization: Bearer $NOTION_TOKEN" \
        -H "Notion-Version: $NOTION_VERSION")"
    fi

    if test "$http_status" = "429"; then
      retry_after="$(awk 'BEGIN{IGNORECASE=1} /^Retry-After:/ {gsub("\r","",$2); print $2}' "$tmp_headers" | tail -1)"
      sleep "${retry_after:-2}"
      continue
    fi
    cat "$tmp_body"
    rm -f "$tmp_headers" "$tmp_body"
    test "$http_status" -ge 200 && test "$http_status" -lt 300
    return
  done

  cat "$tmp_body"
  rm -f "$tmp_headers" "$tmp_body"
  return 1
}
```

For file upload content, call `curl` directly with the same headers.

## Rules

- Always filter API JSON with `jq` before reading or returning results.
- Never print `$NOTION_TOKEN`.
- Always call `$NOTION_API_URL` for API requests. Do not call `https://api.notion.com` directly in runtime work unless the user is explicitly debugging Notion outside Hivy.
- Never construct a human-facing Notion URL from `$NOTION_API_URL`. Use `url` or `public_url` fields from Notion responses, or report the ID when no UI URL is available.
- Use `Notion-Version: 2026-03-11` unless the user explicitly requests another version.
- Prefer the markdown endpoints for broad document reading and editing. Use block endpoints when you need precise block structure, media blocks, tables, toggles, or block IDs.
- Use data source endpoints for table rows and schemas. Current Notion databases are containers; data sources hold the schema and rows.
- Search is title-oriented and not exhaustive. For rows in a known table, query the data source instead of global search.
- Notion access is based on connection capabilities and sharing. `403 restricted_resource` usually means missing capability; `404 object_not_found` often means the page, database, or data source is not shared with the connection.
- Keep requests under limits: average 3 requests/second, `page_size <= 100`, max 100 block children per append, max 1000 block elements and 500KB per payload, max 2000 chars per rich text text node or URL.
- Empty strings are not valid in Notion API JSON. Use `null` to unset values when the endpoint supports it.
- The API has custom emoji endpoints, but no public endpoint for page/comment reactions. If asked to add or list reactions, say that reactions are not exposed by the public REST API.

## Token and workspace check

```bash
notion_api GET "/v1/users/me" \
  | jq '{id, type, name, bot: (.bot // null), workspace_limits: (.bot.workspace_limits // null)}'
```

Personal access tokens cannot list all users. If `/v1/users` returns `403`, use `/v1/users/me` and retrieve known user IDs individually.

## Human-facing links

Return Notion UI links only from Notion object response fields:

```bash
PAGE_ID="..."
notion_api GET "/v1/pages/$PAGE_ID" \
  | jq '{id, title: ([.properties[]? | select(.type == "title") | .title[]?.plain_text] | join("")), url, public_url}'
```

For data sources and databases:

```bash
DATA_SOURCE_ID="..."
notion_api GET "/v1/data_sources/$DATA_SOURCE_ID" \
  | jq '{id, title: ([.title[]?.plain_text] | join("")), url, public_url}'
```

Do not transform `$NOTION_API_URL/v1/pages/...` into a browser link. `NOTION_API_URL` is the Hivy runtime proxy and is not suitable for teammates to open in a browser.

## Search accessible content

```bash
QUERY="roadmap"
notion_api POST "/v1/search" "$(jq -n --arg q "$QUERY" '{
  query: $q,
  filter: {property: "object", value: "page"},
  sort: {timestamp: "last_edited_time", direction: "descending"},
  page_size: 10
}')" | jq '{
  has_more,
  next_cursor,
  pages: [.results[] | {
    id,
    url,
    last_edited_time,
    title: ([.properties[]? | select(.type == "title") | .title[]?.plain_text] | join("")),
    parent
  }]
}'
```

For data sources:

```bash
notion_api POST "/v1/search" "$(jq -n '{filter:{property:"object", value:"data_source"}, page_size: 25}')" \
  | jq '[.results[] | {id, url, title: ([.title[]?.plain_text] | join("")), parent}]'
```

Paginate a POST endpoint:

```bash
CURSOR="paste_next_cursor_here"
notion_api POST "/v1/search" "$(jq -n --arg cursor "$CURSOR" '{start_cursor:$cursor, page_size:100}')" \
  | jq '{has_more, next_cursor, count: (.results | length)}'
```

## Read pages

Retrieve page metadata and properties:

```bash
PAGE_ID="..."
notion_api GET "/v1/pages/$PAGE_ID" \
  | jq '{id, url, created_time, last_edited_time, in_trash, icon, cover, parent, properties}'
```

Read a page as enhanced markdown:

```bash
PAGE_ID="..."
notion_api GET "/v1/pages/$PAGE_ID/markdown" \
  | jq -r '.markdown'
```

Include meeting note transcripts when available:

```bash
notion_api GET "/v1/pages/$PAGE_ID/markdown?include_transcript=true" \
  | jq -r '.markdown'
```

Read block children when structure matters:

```bash
BLOCK_ID="$PAGE_ID"
notion_api GET "/v1/blocks/$BLOCK_ID/children?page_size=100" \
  | jq '{
    has_more,
    next_cursor,
    blocks: [.results[] | {
      id,
      type,
      has_children,
      text: (.[.type].rich_text? // [] | map(.plain_text) | join(""))
    }]
  }'
```

If a block has `has_children: true`, call `/v1/blocks/<child_block_id>/children` recursively.

Retrieve one block:

```bash
BLOCK_ID="..."
notion_api GET "/v1/blocks/$BLOCK_ID" \
  | jq '{id, type, has_children, created_time, last_edited_time, content: .[.type]}'
```

Retrieve one page property item, useful for large relations, rollups, people, or rich text:

```bash
PAGE_ID="..."
PROPERTY_ID="title"
notion_api GET "/v1/pages/$PAGE_ID/properties/$PROPERTY_ID?page_size=100" \
  | jq '{object, type, has_more, next_cursor, results}'
```

## Create pages

Create a top-level workspace page when the token supports it:

```bash
TITLE="New Notion page"
notion_api POST "/v1/pages" "$(jq -n --arg title "$TITLE" '{
  parent: {type: "workspace", workspace: true},
  icon: {type: "emoji", emoji: "\ud83d\udcc4"},
  properties: {
    title: [{type: "text", text: {content: $title}}]
  },
  markdown: "# New Notion page\n\nCreated from the Notion API."
}')" | jq '{id, url}'
```

Create a child page under an existing page. For a page parent, only `title` is valid in `properties`.

```bash
PARENT_PAGE_ID="..."
TITLE="Child page"
notion_api POST "/v1/pages" "$(jq -n --arg parent "$PARENT_PAGE_ID" --arg title "$TITLE" '{
  parent: {type: "page_id", page_id: $parent},
  properties: {
    title: [{type: "text", text: {content: $title}}]
  },
  children: [
    {type: "heading_2", heading_2: {rich_text: [{type: "text", text: {content: "Summary"}}]}},
    {type: "paragraph", paragraph: {rich_text: [{type: "text", text: {content: "Body text."}}]}}
  ]
}')" | jq '{id, url}'
```

Create a row/page under a data source:

```bash
DATA_SOURCE_ID="..."
notion_api POST "/v1/pages" "$(jq -n --arg ds "$DATA_SOURCE_ID" '{
  parent: {type: "data_source_id", data_source_id: $ds},
  properties: {
    Name: {title: [{type: "text", text: {content: "Ship API migration"}}]},
    Status: {select: {name: "To Do"}},
    Due: {date: {start: "2026-06-01"}}
  }
}')" | jq '{id, url, properties}'
```

Create from a data source template:

```bash
notion_api POST "/v1/pages" "$(jq -n --arg ds "$DATA_SOURCE_ID" '{
  parent: {type: "data_source_id", data_source_id: $ds},
  properties: {Name: {title: [{text: {content: "Templated page"}}]}},
  template: {type: "default"}
}')" | jq '{id, url}'
```

Do not include `children` when applying a template. Template application is asynchronous.

## Edit pages with markdown

For new integrations, prefer `update_content` for targeted search-and-replace and `replace_content` for full rewrites. `insert_content` and `replace_content_range` are legacy commands, but still useful for start/end insertion.

Search and replace exact markdown text:

```bash
PAGE_ID="..."
notion_api PATCH "/v1/pages/$PAGE_ID/markdown" "$(jq -n '{
  type: "update_content",
  update_content: {
    content_updates: [
      {old_str: "Draft proposal", new_str: "Draft proposal (due Friday)"}
    ]
  }
}')" | jq -r '.markdown'
```

Replace every matching occurrence:

```bash
notion_api PATCH "/v1/pages/$PAGE_ID/markdown" "$(jq -n '{
  type: "update_content",
  update_content: {
    content_updates: [
      {old_str: "TODO", new_str: "Done", replace_all_matches: true}
    ]
  }
}')" | jq -r '.markdown'
```

Replace the whole page body:

```bash
notion_api PATCH "/v1/pages/$PAGE_ID/markdown" "$(jq -n '{
  type: "replace_content",
  replace_content: {
    new_str: "# Rewritten\n\nThis replaces the page content."
  }
}')" | jq -r '.markdown'
```

Append markdown to the end with the legacy insert command:

```bash
PAGE_ID="..."
notion_api PATCH "/v1/pages/$PAGE_ID/markdown" "$(jq -n '{
  type: "insert_content",
  insert_content: {
    content: "## Update\n\n- Added by API\n- Uses enhanced markdown",
    position: {type: "end"}
  }
}')" | jq -r '.markdown'
```

Insert at the start:

```bash
notion_api PATCH "/v1/pages/$PAGE_ID/markdown" "$(jq -n '{
  type: "insert_content",
  insert_content: {
    content: "> Prepended note",
    position: {type: "start"}
  }
}')" | jq -r '.markdown'
```

Insert after a matched markdown range:

```bash
notion_api PATCH "/v1/pages/$PAGE_ID/markdown" "$(jq -n --arg after "Heading...last sentence" '{
  type: "insert_content",
  insert_content: {
    content: "\nInserted after the selected range.",
    after: $after
  }
}')" | jq -r '.markdown'
```

Replace a matched range with the legacy range command:

```bash
notion_api PATCH "/v1/pages/$PAGE_ID/markdown" "$(jq -n '{
  type: "replace_content_range",
  replace_content_range: {
    content: "## Replacement\n\nNew content.",
    content_range: "Old heading...Old final line"
  }
}')" | jq -r '.markdown'
```

Markdown matching is case-sensitive. `update_content.old_str` must match exactly one location unless `replace_all_matches: true` is set. Do not provide both `insert_content.after` and `insert_content.position`.

## Edit page properties, icon, cover, trash, move

```bash
PAGE_ID="..."
TITLE="Updated title"
notion_api PATCH "/v1/pages/$PAGE_ID" "$(jq -n --arg title "$TITLE" '{
  icon: {type: "emoji", emoji: "\u2705"},
  properties: {
    title: [{type: "text", text: {content: $title}}]
  }
}')" | jq '{id, url, icon, properties}'
```

External cover:

```bash
notion_api PATCH "/v1/pages/$PAGE_ID" "$(jq -n '{
  cover: {type: "external", external: {url: "https://www.notion.so/images/page-cover/woodcuts_1.jpg"}}
}')" | jq '{id, cover}'
```

Trash or restore:

```bash
notion_api PATCH "/v1/pages/$PAGE_ID" '{"in_trash":true}' | jq '{id, in_trash}'
notion_api PATCH "/v1/pages/$PAGE_ID" '{"in_trash":false}' | jq '{id, in_trash}'
```

Move a page:

```bash
NEW_PARENT_PAGE_ID="..."
notion_api POST "/v1/pages/$PAGE_ID/move" "$(jq -n --arg parent "$NEW_PARENT_PAGE_ID" '{
  parent: {type: "page_id", page_id: $parent}
}')" | jq '{id, parent, url}'
```

Move a row/page into a data source:

```bash
NEW_DATA_SOURCE_ID="..."
notion_api POST "/v1/pages/$PAGE_ID/move" "$(jq -n --arg ds "$NEW_DATA_SOURCE_ID" '{
  parent: {type: "data_source_id", data_source_id: $ds}
}')" | jq '{id, parent, url}'
```

Do not use a workspace parent with `/v1/pages/{page_id}/move`; workspace parent is valid for creating a top-level page, not for moving a page.

## Edit blocks

Append blocks at the end, start, or after a block. Use `position`; older `after` is deprecated in `2026-03-11`.

```bash
BLOCK_ID="$PAGE_ID"
notion_api PATCH "/v1/blocks/$BLOCK_ID/children" "$(jq -n '{
  children: [
    {type: "heading_2", heading_2: {rich_text: [{text: {content: "API section"}}]}},
    {type: "to_do", to_do: {rich_text: [{text: {content: "Check the result"}}], checked: false}},
    {type: "callout", callout: {
      icon: {type: "emoji", emoji: "\ud83d\udca1"},
      rich_text: [{text: {content: "Callouts can have emoji, custom emoji, native icons, or files."}}]
    }}
  ],
  position: {type: "end"}
}')" | jq '[.results[] | {id, type}]'
```

Append an external image:

```bash
notion_api PATCH "/v1/blocks/$PAGE_ID/children" "$(jq -n '{
  children: [
    {type: "image", image: {type: "external", external: {url: "https://www.notion.so/images/page-cover/gradients_10.jpg"}}}
  ]
}')" | jq '[.results[] | {id, type, image}]'
```

Update a block:

```bash
BLOCK_ID="..."
notion_api PATCH "/v1/blocks/$BLOCK_ID" "$(jq -n '{
  paragraph: {
    rich_text: [{type: "text", text: {content: "Updated paragraph text"}}]
  }
}')" | jq '{id, type, content: .[.type]}'
```

Delete a block:

```bash
BLOCK_ID="..."
notion_api DELETE "/v1/blocks/$BLOCK_ID" | jq '{id, type, in_trash}'
```

Common block types: `paragraph`, `heading_1`, `heading_2`, `heading_3`, `bulleted_list_item`, `numbered_list_item`, `to_do`, `toggle`, `quote`, `callout`, `code`, `divider`, `image`, `file`, `video`, `pdf`, `embed`, `bookmark`, `table`, `table_row`, `column_list`, `column`, `synced_block`, `meeting_notes`.

## Rich text, mentions, emojis, icons

Rich text object:

```json
{
  "type": "text",
  "text": {"content": "linked text", "link": {"url": "https://developers.notion.com/"}},
  "annotations": {"bold": true, "italic": false, "strikethrough": false, "underline": false, "code": false, "color": "default"}
}
```

Mentions:

```json
{"type":"mention","mention":{"type":"page","page":{"id":"PAGE_ID"}}}
{"type":"mention","mention":{"type":"user","user":{"object":"user","id":"USER_ID"}}}
{"type":"mention","mention":{"type":"date","date":{"start":"2026-05-18","end":null}}}
```

Emoji page icon:

```json
{"icon":{"type":"emoji","emoji":"\ud83d\ude80"}}
```

Native Notion icon:

```json
{"icon":{"type":"icon","icon":{"name":"pizza","color":"blue"}}}
```

List custom emojis:

```bash
notion_api GET "/v1/custom_emojis?page_size=100" \
  | jq '[.results[] | {id, name, url}]'
```

Find one custom emoji by name:

```bash
EMOJI_NAME="ship"
notion_api GET "/v1/custom_emojis?name=$EMOJI_NAME" \
  | jq '[.results[] | {id, name, url}]'
```

Use a custom emoji icon only after you have its ID:

```json
{"icon":{"type":"custom_emoji","custom_emoji":{"id":"CUSTOM_EMOJI_ID"}}}
```

## Databases and data sources

Retrieve a database container and its data sources:

```bash
DATABASE_ID="..."
notion_api GET "/v1/databases/$DATABASE_ID" \
  | jq '{id, url, title: ([.title[]?.plain_text] | join("")), data_sources}'
```

Retrieve a data source schema:

```bash
DATA_SOURCE_ID="..."
notion_api GET "/v1/data_sources/$DATA_SOURCE_ID" \
  | jq '{id, url, title: ([.title[]?.plain_text] | join("")), parent, properties}'
```

Create a database and initial data source under a page:

```bash
PARENT_PAGE_ID="..."
notion_api POST "/v1/databases" "$(jq -n --arg parent "$PARENT_PAGE_ID" '{
  parent: {type: "page_id", page_id: $parent},
  title: [{type: "text", text: {content: "Tasks"}}],
  initial_data_source: {
    properties: {
      Name: {title: {}},
      Status: {select: {options: [{name: "To Do", color: "red"}, {name: "Done", color: "green"}]}},
      Due: {date: {}},
      Done: {checkbox: {}}
    }
  }
}')" | jq '{id, url, data_sources}'
```

Create an additional data source under a database:

```bash
notion_api POST "/v1/data_sources" "$(jq -n --arg db "$DATABASE_ID" '{
  parent: {type: "database_id", database_id: $db},
  title: [{type: "text", text: {content: "Bugs"}}],
  properties: {
    Name: {title: {}},
    Priority: {select: {options: [{name: "High", color: "red"}, {name: "Low", color: "gray"}]}}
  }
}')" | jq '{id, title, parent, properties}'
```

Query a data source:

```bash
DATA_SOURCE_ID="..."
notion_api POST "/v1/data_sources/$DATA_SOURCE_ID/query?filter_properties[]=Name&filter_properties[]=Status" "$(jq -n '{
  filter: {property: "Status", select: {equals: "To Do"}},
  sorts: [{property: "Due", direction: "ascending"}],
  page_size: 25
}')" | jq '{
  has_more,
  next_cursor,
  rows: [.results[] | {
    id,
    url,
    title: ([.properties.Name.title[]?.plain_text] | join("")),
    status: (.properties.Status.select.name // null),
    due: (.properties.Due.date.start // null)
  }]
}'
```

Update data source schema:

```bash
notion_api PATCH "/v1/data_sources/$DATA_SOURCE_ID" "$(jq -n '{
  properties: {
    Effort: {number: {format: "number"}},
    OldProperty: null,
    Status: {name: "State"}
  }
}')" | jq '{id, properties}'
```

List data source templates:

```bash
notion_api GET "/v1/data_sources/$DATA_SOURCE_ID/templates?page_size=100" \
  | jq '[.results[] | {id, name, created_time, last_edited_time}]'
```

## Comments

List unresolved comments for a page or block:

```bash
BLOCK_OR_PAGE_ID="..."
notion_api GET "/v1/comments?block_id=$BLOCK_OR_PAGE_ID&page_size=100" \
  | jq '{has_more, next_cursor, comments: [.results[] | {id, discussion_id, created_time, rich_text: [.rich_text[]?.plain_text] | join("")}]}'
```

Create a comment on a page:

```bash
PAGE_ID="..."
notion_api POST "/v1/comments" "$(jq -n --arg page "$PAGE_ID" '{
  parent: {page_id: $page},
  rich_text: [{type: "text", text: {content: "Comment created through the Notion API."}}]
}')" | jq '{id, discussion_id, created_time, rich_text}'
```

Reply to an existing discussion:

```bash
DISCUSSION_ID="..."
notion_api POST "/v1/comments" "$(jq -n --arg discussion "$DISCUSSION_ID" '{
  discussion_id: $discussion,
  rich_text: [{type: "text", text: {content: "Reply from the API."}}]
}')" | jq '{id, discussion_id, rich_text}'
```

Update a comment created by this connection:

```bash
COMMENT_ID="..."
notion_api PATCH "/v1/comments/$COMMENT_ID" "$(jq -n '{
  rich_text: [{type: "text", text: {content: "Updated API comment."}}]
}')" | jq '{id, rich_text}'
```

Delete a comment created by this connection:

```bash
COMMENT_ID="..."
notion_api DELETE "/v1/comments/$COMMENT_ID" | jq '{id, in_trash}'
```

Create comment with an uploaded file attachment:

```json
{
  "parent": {"page_id": "PAGE_ID"},
  "rich_text": [{"text": {"content": "See attached."}}],
  "attachments": [{"type": "file_upload", "file_upload_id": "FILE_UPLOAD_ID"}]
}
```

Comments require comment capabilities. The API cannot create new inline discussion threads. Attachments are limited to 3.

## Files and media

List file uploads created by the current connection:

```bash
notion_api GET "/v1/file_uploads?page_size=100" \
  | jq '[.results[] | {id, status, filename, content_type, created_time}]'
```

Single-part upload up to 20 MiB:

```bash
FILE_PATH="./report.pdf"
FILE_NAME="$(basename "$FILE_PATH")"
CONTENT_TYPE="application/pdf"
UPLOAD_ID="$(notion_api POST "/v1/file_uploads" "$(jq -n --arg filename "$FILE_NAME" --arg content_type "$CONTENT_TYPE" '{
  mode: "single_part",
  filename: $filename,
  content_type: $content_type
}')" | jq -r '.id')"

curl -fsS -X POST "$NOTION_API_URL/v1/file_uploads/$UPLOAD_ID/send" \
  -H "Authorization: Bearer $NOTION_TOKEN" \
  -H "Notion-Version: $NOTION_VERSION" \
  -F "file=@$FILE_PATH" \
  | jq '{id, status, filename, content_type}'
```

Attach uploaded file as a file block:

```bash
notion_api PATCH "/v1/blocks/$PAGE_ID/children" "$(jq -n --arg upload "$UPLOAD_ID" '{
  children: [
    {type: "file", file: {type: "file_upload", file_upload: {id: $upload}}}
  ]
}')" | jq '[.results[] | {id, type, file}]'
```

External files do not require upload if they are public URLs:

```bash
notion_api PATCH "/v1/blocks/$PAGE_ID/children" "$(jq -n '{
  children: [
    {type: "file", file: {type: "external", external: {url: "https://example.com/report.pdf"}}}
  ]
}')" | jq '[.results[] | {id, type, file}]'
```

Notion-hosted file URLs expire. Fetch the page, block, or file again to get a fresh URL.

## Users

Current token:

```bash
notion_api GET "/v1/users/me" | jq '{id, type, name, person, bot}'
```

List users when supported by token type and user capabilities. Personal access tokens return `403 restricted_resource` for this endpoint.

```bash
notion_api GET "/v1/users?page_size=100" \
  | jq '{has_more, next_cursor, users: [.results[] | {id, type, name, email: (.person.email // null)}]}'
```

Retrieve a known user when supported by token type and user capabilities. Personal access tokens may only support `/v1/users/me`.

```bash
USER_ID="..."
notion_api GET "/v1/users/$USER_ID" | jq '{id, type, name, email: (.person.email // null), bot}'
```

## Troubleshooting

Inspect a Notion error:

```bash
notion_api GET "/v1/pages/$PAGE_ID" \
  | jq 'if .object == "error" then {status, code, message, additional_data} else . end'
```

Common responses:

| Status/code | Meaning | Action |
|---|---|---|
| `400 missing_version` | Missing `Notion-Version` | Use the helper or add the header |
| `400 validation_error` | Body shape or size is invalid | Read `.message`, fix schema, split large requests |
| `401 unauthorized` | Token invalid | Check runtime connection |
| `403 restricted_resource` | Missing capability or token type restriction | Use a supported endpoint or update connection capabilities |
| `404 object_not_found` | Missing object or not shared with connection | Share the page/database or verify the ID |
| `409 conflict_error` | Save collision or temporary file storage issue | Re-read and retry |
| `429 rate_limited` | Average request rate exceeded | Respect `Retry-After` |
| `502/503/504` | Notion temporary failure or timeout | Retry with backoff |

## API discovery

When this skill does not cover an operation, check the official docs or OpenAPI before guessing. Use these docs only for reference; runtime API calls still go through `$NOTION_API_URL`.

```bash
curl -fsS https://developers.notion.com/openapi.json -o /tmp/notion-openapi.json
jq -r '.paths | keys[]' /tmp/notion-openapi.json
jq '.paths["/v1/pages"].post.requestBody.content["application/json"].schema' /tmp/notion-openapi.json
```

Key official docs:

- Introduction: `https://developers.notion.com/reference/intro`
- Versioning: `https://developers.notion.com/reference/versioning`
- Request limits: `https://developers.notion.com/reference/request-limits`
- Status codes: `https://developers.notion.com/reference/status-codes`
- Pages: `https://developers.notion.com/reference/page`
- Blocks: `https://developers.notion.com/reference/block`
- Markdown content: `https://developers.notion.com/guides/data-apis/working-with-markdown-content`
- Databases: `https://developers.notion.com/reference/database`
- Data sources: `https://developers.notion.com/reference/data-source`
- Comments: `https://developers.notion.com/reference/comment-object`
- Files: `https://developers.notion.com/reference/file-object`
- File uploads: `https://developers.notion.com/reference/file-upload`
- Rich text: `https://developers.notion.com/reference/rich-text`
- Emoji and icon: `https://developers.notion.com/reference/emoji-and-icon`
