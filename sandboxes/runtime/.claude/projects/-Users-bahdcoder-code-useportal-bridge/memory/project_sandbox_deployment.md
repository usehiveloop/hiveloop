---
name: Bridge runs in isolated sandboxes
description: Bridge deploys in isolated sandboxes with workspace dir pointing to user's repo — no home directory access, no global config
type: project
---

Bridge mostly runs in isolated sandboxes. The workspace directory points to the user's repo. No user-global config paths (~/.claude/) are available at runtime.

**Why:** Deployment model is containerized/sandboxed — no shared home directory.
**How to apply:** Never design features that depend on ~/.claude/ or other home-directory paths. Always scope filesystem access to the working directory only.
