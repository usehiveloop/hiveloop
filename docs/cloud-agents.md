# Cloud Agents API

Cloud agents are subagents that run in their own dedicated sandbox. An employee agent can launch long-running tasks to cloud agents, query their progress, and communicate with them.

All endpoints are bridge-authenticated — the caller must be inside a sandbox and present the sandbox's bridge API key as a bearer token.

## Authentication

Every request must include the employee sandbox's bridge API key:

```
Authorization: Bearer <bridge-api-key>
```

The bridge API key is provisioned when the employee's sandbox is created. Requests from outside a sandbox, or with an invalid key, receive `401 Unauthorized`.

---

## Endpoints

### List Cloud Agents

Returns all cloud agents owned by the employee, each with their 3 most recent tasks and 10 most recent events per task.

```
GET /internal/employees/{employeeID}/cloud-agents/
```

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| `employeeID` | UUID | The employee agent's ID. Must match the sandbox owner. |

**Response: `200 OK`**

```json
{
  "cloud_agents": [
    {
      "id": "a1b2c3d4-...",
      "name": "code-worker",
      "system_prompt": "You are a senior software engineer...",
      "model": "anthropic/claude-sonnet-4-20250514",
      "tools": ["Read", "write", "edit", "bash", "..."],
      "skills": ["git-github", "agent-browser"],
      "recent_tasks": [
        {
          "id": "t1u2v3w4-...",
          "brief": "Refactor the auth middleware to use JWT",
          "metadata": { "source": "slack", "channel": "C123" },
          "conversation_id": "c5d6e7f8-...",
          "sandbox_id": "s9a0b1c2-...",
          "created_at": "2026-05-08T10:30:00Z",
          "recent_events": [
            {
              "event_type": "message_received",
              "data": { "content": "Starting the refactor..." },
              "created_at": "2026-05-08T10:30:05Z"
            },
            {
              "event_type": "tool_call",
              "data": { "tool": "Read", "input": { "file": "auth.go" } },
              "created_at": "2026-05-08T10:30:10Z"
            }
          ]
        }
      ]
    }
  ]
}
```

**Error Responses**

| Status | Body | Cause |
|--------|------|-------|
| `400` | `{"error": "invalid employee_id"}` | Malformed UUID in path |
| `401` | `{"error": "missing authorization"}` | No `Authorization` header |
| `401` | `{"error": "invalid bridge api key"}` | Bearer doesn't match sandbox key |
| `404` | `{"error": "employee not found"}` | No employee agent with this ID |
| `404` | `{"error": "sandbox not found for employee"}` | Employee has no active sandbox |

---

### List Tasks

Returns paginated tasks for a specific cloud agent, newest first.

```
GET /internal/employees/{employeeID}/cloud-agents/{agentID}/tasks
```

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| `employeeID` | UUID | The employee agent's ID. |
| `agentID` | UUID | The cloud agent's ID. Must be a subagent of this employee. |

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | int | 50 | Results per page (max 100). |
| `cursor` | string | — | Pagination cursor from a previous response. |

**Response: `200 OK`**

```json
{
  "data": [
    {
      "id": "t1u2v3w4-...",
      "cloud_agent_id": "a1b2c3d4-...",
      "sandbox_id": "s9a0b1c2-...",
      "conversation_id": "c5d6e7f8-...",
      "parent_conversation_type": "agent_conversation",
      "parent_conversation_id": "p1q2r3s4-...",
      "brief": "Refactor the auth middleware to use JWT",
      "metadata": { "source": "slack" },
      "created_at": "2026-05-08T10:30:00Z"
    }
  ],
  "next_cursor": "MjAyNi0wNS0wOFQxMDozMDowMFp8dDF1MnYzdzQt...",
  "has_more": true
}
```

**Error Responses**

| Status | Body | Cause |
|--------|------|-------|
| `400` | `{"error": "invalid agent_id"}` | Malformed UUID in path |
| `400` | `{"error": "invalid cursor"}` | Malformed pagination cursor |
| `401` | `{"error": "missing authorization"}` | No `Authorization` header |
| `401` | `{"error": "invalid bridge api key"}` | Bearer doesn't match sandbox key |
| `404` | `{"error": "employee not found"}` | No employee agent with this ID |
| `404` | `{"error": "cloud agent not found for this employee"}` | Agent is not a subagent of this employee |

---

### Get Task

Returns a single task by ID.

```
GET /internal/employees/{employeeID}/cloud-agents/{agentID}/tasks/{taskID}
```

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| `employeeID` | UUID | The employee agent's ID. |
| `agentID` | UUID | The cloud agent's ID. |
| `taskID` | UUID | The task's ID. |

**Response: `200 OK`**

```json
{
  "id": "t1u2v3w4-...",
  "cloud_agent_id": "a1b2c3d4-...",
  "sandbox_id": "s9a0b1c2-...",
  "conversation_id": "c5d6e7f8-...",
  "parent_conversation_type": "agent_conversation",
  "parent_conversation_id": "p1q2r3s4-...",
  "brief": "Refactor the auth middleware to use JWT",
  "metadata": { "source": "slack" },
  "created_at": "2026-05-08T10:30:00Z"
}
```

**Error Responses**

| Status | Body | Cause |
|--------|------|-------|
| `400` | `{"error": "invalid agent_id"}` | Malformed UUID in path |
| `400` | `{"error": "invalid task_id"}` | Malformed UUID in path |
| `401` | `{"error": "missing authorization"}` | No `Authorization` header |
| `401` | `{"error": "invalid bridge api key"}` | Bearer doesn't match sandbox key |
| `404` | `{"error": "employee not found"}` | No employee agent with this ID |
| `404` | `{"error": "cloud agent not found for this employee"}` | Agent is not a subagent |
| `404` | `{"error": "task not found"}` | No task with this ID for this agent |

---

### Create Task

Synchronously provisions a new sandbox for the cloud agent, injects the agent, creates a conversation, sends the brief as the first message, and returns the task ID. This call blocks until the sandbox is live and the agent is ready.

```
POST /internal/employees/{employeeID}/cloud-agents/{agentID}/tasks
```

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| `employeeID` | UUID | The employee agent's ID. |
| `agentID` | UUID | The cloud agent's ID. Must be a subagent of this employee. |

**Request Body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `brief` | string | Yes | The task instructions. Sent as the first message to the cloud agent. |
| `parent_conversation_type` | string | Yes | Type of the parent conversation. One of: `agent_conversation`, `chat_session`, `external`. |
| `parent_conversation_id` | string | Yes | ID of the parent conversation. Used to deliver completion callbacks. |
| `metadata` | object | No | Arbitrary key-value data stored with the task. Returned in list/get responses. |

**Example Request**

```json
{
  "brief": "Refactor the auth middleware to use JWT instead of sessions. Here is the current implementation: ...",
  "parent_conversation_type": "agent_conversation",
  "parent_conversation_id": "p1q2r3s4-...",
  "metadata": {
    "source": "slack",
    "channel": "C123",
    "thread_ts": "1234.5678"
  }
}
```

**Response: `201 Created`**

```json
{
  "task_id": "t1u2v3w4-...",
  "message": "You may use the tool cloud_agent_task_status(t1u2v3w4-...) to get progress events from the task, and the tool cloud_agent_task_send_message to send messages to this agent."
}
```

The response returns immediately after the sandbox is provisioned and the brief is sent. The cloud agent will begin working on the task asynchronously. Use the `task_id` and `conversation_id` (from `GET /tasks/{taskID}`) to monitor progress via events.

**Error Responses**

| Status | Body | Cause |
|--------|------|-------|
| `400` | `{"error": "invalid agent_id"}` | Malformed UUID in path |
| `400` | `{"error": "invalid request body"}` | Malformed JSON body |
| `400` | `{"error": "brief is required"}` | `brief` is empty |
| `400` | `{"error": "parent_conversation_type and parent_conversation_id are required"}` | Missing parent info |
| `401` | `{"error": "missing authorization"}` | No `Authorization` header |
| `401` | `{"error": "invalid bridge api key"}` | Bearer doesn't match sandbox key |
| `404` | `{"error": "employee not found"}` | No employee agent with this ID |
| `404` | `{"error": "cloud agent not found for this employee"}` | Agent is not a subagent |
| `404` | `{"error": "cloud agent not found"}` | No agent with this ID exists |
| `503` | `{"error": "sandbox orchestrator not configured"}` | Sandbox infra unavailable |
| `500` | `{"error": "failed to provision sandbox"}` | Sandbox creation failed |
| `500` | `{"error": "failed to initialize agent in sandbox"}` | Agent injection failed |
| `500` | `{"error": "failed to connect to sandbox"}` | Bridge client connection failed |
| `500` | `{"error": "failed to create conversation"}` | Bridge conversation creation failed |
| `500` | `{"error": "failed to send task brief"}` | Message delivery failed |

---

## Data Model

### CloudAgentTask

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID | Unique task identifier. |
| `cloud_agent_id` | UUID | The cloud agent running this task. |
| `sandbox_id` | UUID | The dedicated sandbox for this task. |
| `conversation_id` | UUID | The `AgentConversation` backing this task. Use this to query events. |
| `parent_conversation_type` | string | Where to send the completion callback. |
| `parent_conversation_id` | string | The parent conversation to notify on completion. |
| `brief` | string | The task instructions sent to the agent. |
| `metadata` | object | Arbitrary caller-provided context. |
| `created_at` | ISO 8601 | When the task was created. |

### Lifecycle

Tasks are created synchronously and are immutable after creation. There is no status field — the agent's state is determined by querying `ConversationEvent` records for the task's `conversation_id`:

- **Running**: conversation exists, events are being produced
- **Completed**: a `ConversationEnded` event exists for the conversation
- **Failed**: an `AgentError` event exists, or the sandbox entered an error state

When the cloud agent's conversation ends, a callback message is automatically delivered to the parent conversation (identified by `parent_conversation_type` and `parent_conversation_id`).

---

## Usage from an Employee Agent

From inside the sandbox, the employee agent interacts with cloud agents through these HTTP endpoints. A typical flow:

1. **List available cloud agents**: `GET /internal/employees/{self}/cloud-agents/`
2. **Launch a task**: `POST /internal/employees/{self}/cloud-agents/{agentID}/tasks` with a detailed brief
3. **Check progress**: `GET /internal/employees/{self}/cloud-agents/{agentID}/tasks/{taskID}` or list events
4. **Completion callback**: The cloud agent's `ConversationEnded` event triggers an automatic message to the parent conversation
