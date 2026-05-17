type AgentManifestInput = {
  name: string
  description?: string
}

const SLACK_MANIFEST_TEMPLATE = {
  display_information: {
    name: "Hiveloop Employee",
    description: "Your Hiveloop employee on Slack",
    background_color: "#9f2d00",
  },
  features: {
    bot_user: {
      display_name: "Hiveloop Employee",
      always_online: true,
    },
    assistant_view: {
      assistant_description: "Chat with your Hiveloop employee in threads and DMs.",
      suggested_prompts: [],
    },
  },
  oauth_config: {
    scopes: {
      bot: [
        "app_mentions:read",
        "assistant:write",
        "channels:history",
        "channels:join",
        "channels:read",
        "chat:write",
        "commands",
        "emoji:read",
        "files:read",
        "files:write",
        "groups:history",
        "groups:read",
        "groups:write",
        "im:history",
        "im:read",
        "im:write",
        "mpim:history",
        "mpim:read",
        "mpim:write",
        "pins:read",
        "pins:write",
        "reactions:read",
        "reactions:write",
        "users:read",
        "users:read.email",
      ],
    },
  },
  settings: {
    event_subscriptions: {
      bot_events: [
        "app_mention",
        "assistant_thread_context_changed",
        "assistant_thread_started",
        "channel_rename",
        "member_joined_channel",
        "member_left_channel",
        "message.channels",
        "message.groups",
        "message.im",
        "message.mpim",
        "pin_added",
        "pin_removed",
        "reaction_added",
        "reaction_removed",
      ],
    },
    interactivity: {
      is_enabled: true,
    },
    org_deploy_enabled: false,
    socket_mode_enabled: true,
    token_rotation_enabled: false,
  },
} as const

// Slack's manifest schema caps display_information.description at 140 chars.
// long_description fits up to 4000 (and requires >=50 when set).
const SLACK_DESCRIPTION_MAX = 135

export function buildSlackAppManifest({ name, description }: AgentManifestInput) {
  const trimmedName = name.trim() || "Hiveloop Employee"
  const trimmedDescription = description?.trim() || `Your ${trimmedName} agent on Slack`
  const overflows = trimmedDescription.length > SLACK_DESCRIPTION_MAX
  const shortDescription = overflows
    ? trimmedDescription.slice(0, SLACK_DESCRIPTION_MAX)
    : trimmedDescription

  return {
    ...SLACK_MANIFEST_TEMPLATE,
    display_information: {
      ...SLACK_MANIFEST_TEMPLATE.display_information,
      name: trimmedName,
      description: shortDescription,
      ...(overflows ? { long_description: trimmedDescription } : {}),
    },
    features: {
      ...SLACK_MANIFEST_TEMPLATE.features,
      bot_user: {
        ...SLACK_MANIFEST_TEMPLATE.features.bot_user,
        display_name: trimmedName,
      },
      assistant_view: {
        ...SLACK_MANIFEST_TEMPLATE.features.assistant_view,
        assistant_description: trimmedDescription,
      },
    },
  }
}

export function buildSlackAppCreateUrl(input: AgentManifestInput): string {
  const manifest = buildSlackAppManifest(input)
  const encoded = encodeURIComponent(JSON.stringify(manifest))
  return `https://api.slack.com/apps?new_app=1&manifest_json=${encoded}`
}
