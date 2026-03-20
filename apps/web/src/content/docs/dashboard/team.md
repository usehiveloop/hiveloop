---
title: Team Management
description: Inviting members, roles, and managing workspace access
---

# Team Management

Manage workspace access by inviting team members and assigning roles.

## Team Members List

Navigate to **Settings > Team** to view members.

### List View

The members table displays:

| Column | Description |
|--------|-------------|
| Name | Full name |
| Email | Email address |
| Role | Permission badge |
| Joined | Date added |
| Actions | Remove button (if applicable) |

### Searching

Search members by:
- Name
- Email address

## Inviting Members

Click **"Invite Member"** to add a new team member.

### Invitation Form

**Required Fields:**

| Field | Description | Example |
|-------|-------------|---------|
| Email | Member's email address | `colleague@company.com` |
| First Name | Given name | `Jane` |
| Last Name | Family name | `Doe` |
| Role | Permission level | Admin or Member |

### Role Descriptions

| Role | Permissions |
|------|-------------|
| **Admin** | Full access to all resources - credentials, tokens, identities, integrations, billing, team management |
| **Member** | Read-only access - can view resources but cannot create, modify, or delete |

### Invitation Process

1. Fill in member details
2. Select appropriate role
3. Click **"Send Invite"**
4. Member receives invitation email
5. Member accepts and joins workspace

## Managing Members

### Changing Roles

Role changes are coming soon. In the meantime, contact support if you need to change a member's role.

### Removing Members

To remove a member:

1. Find the member in the list
2. Click **"Remove"** in the Actions column
3. Confirm removal

**Owner Protection:**
- Owners cannot be removed via the UI
- At least one owner must remain
- Contact support to transfer ownership

## Role Permissions Matrix

| Action | Owner | Admin | Member |
|--------|-------|-------|--------|
| View credentials | Yes | Yes | Yes |
| Create credentials | Yes | Yes | No |
| Revoke credentials | Yes | Yes | No |
| View tokens | Yes | Yes | Yes |
| Mint tokens | Yes | Yes | No |
| Revoke tokens | Yes | Yes | No |
| View identities | Yes | Yes | Yes |
| Create identities | Yes | Yes | No |
| Delete identities | Yes | Yes | No |
| View integrations | Yes | Yes | Yes |
| Add integrations | Yes | Yes | No |
| Delete integrations | Yes | Yes | No |
| View audit log | Yes | Yes | Yes |
| View billing | Yes | Yes | No |
| Change plan | Yes | Yes | No |
| Invite members | Yes | Yes | No |
| Remove members | Yes | Yes | No |
| Delete workspace | Yes | No | No |

## Multiple Organizations

Users can belong to multiple LLMVault organizations:
- Each organization has separate billing
- Members are managed per-organization
- Role can vary by organization
- Switch organizations via workspace switcher

## Security Considerations

1. **Principle of least privilege** - Start with Member role
2. **Regular audits** - Review membership quarterly
3. **Prompt removal** - Remove departing employees immediately
4. **Owner succession** - Ensure multiple owners or documented transfer process

## Member Limits

| Plan | Member Limit |
|------|--------------|
| Free | 3 members |
| Pro | 10 members |
| Enterprise | Unlimited |

Contact sales to increase limits.
