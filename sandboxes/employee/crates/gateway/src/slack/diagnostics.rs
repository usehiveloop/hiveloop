pub fn user_facing_explanation<E: std::fmt::Display>(error: &E) -> Option<String> {
    let message = error.to_string();
    let lowered = message.to_lowercase();

    if lowered.contains("missing_scope") {
        return Some(format!(
            "Slack rejected the call because the bot is missing a required scope. Update the app and reinstall. Raw: {message}"
        ));
    }
    if lowered.contains("not_authed") || lowered.contains("invalid_auth") {
        return Some(format!(
            "Slack rejected the bot token (not_authed/invalid_auth). Re-issue the xoxb token. Raw: {message}"
        ));
    }
    if lowered.contains("token_revoked") {
        return Some(format!(
            "Slack token has been revoked. Reinstall the app. Raw: {message}"
        ));
    }
    if lowered.contains("access_denied")
        || lowered.contains("permission_denied")
        || lowered.contains("not_in_channel")
    {
        return Some(format!(
            "Bot lacks permission for that channel or resource. Invite the bot or add scopes. Raw: {message}"
        ));
    }
    if lowered.contains("rate_limited") || lowered.contains("ratelimited") {
        return Some(format!(
            "Slack rate-limited the call; retry with backoff. Raw: {message}"
        ));
    }
    if lowered.contains("channel_not_found") {
        return Some(format!(
            "Channel not found or bot has no visibility. Raw: {message}"
        ));
    }
    if lowered.contains("thread_not_found") || lowered.contains("message_not_found") {
        return Some(format!(
            "Slack could not locate the parent thread/message. Raw: {message}"
        ));
    }
    None
}
