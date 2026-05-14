use domain::{AllowBotsMode, SlackConfig};

pub enum MentionDecision {
    Allow,
    Block,
}

pub enum BotMessageDecision {
    AllowAlways,
    Drop,
    AllowOnlyIfMentioned,
}

pub fn channel_is_allowed(config: &SlackConfig, channel: &str) -> bool {
    if config.allowed_channels.is_empty() {
        return true;
    }
    config.allowed_channels.iter().any(|c| c == channel)
}

pub fn ignore_users_blocks(config: &SlackConfig, user: &str) -> bool {
    config.ignore_users.iter().any(|u| u == user)
}

pub fn classify_bot_message(config: &SlackConfig, sender_is_bot: bool) -> BotMessageDecision {
    if !sender_is_bot {
        return BotMessageDecision::AllowAlways;
    }
    match config.allow_bots {
        AllowBotsMode::None => BotMessageDecision::Drop,
        AllowBotsMode::Mentions => BotMessageDecision::AllowOnlyIfMentioned,
        AllowBotsMode::All => BotMessageDecision::AllowAlways,
    }
}

pub fn mention_rules_allow(
    config: &SlackConfig,
    is_direct_message: bool,
    channel: &str,
    mention_present: bool,
    thread_already_engaged: bool,
    bot_has_replied_in_thread: bool,
) -> MentionDecision {
    if is_direct_message {
        return MentionDecision::Allow;
    }
    if config.free_response_channels.iter().any(|c| c == channel) {
        return MentionDecision::Allow;
    }
    if config.strict_mention {
        return if mention_present {
            MentionDecision::Allow
        } else {
            MentionDecision::Block
        };
    }
    if !config.require_mention {
        return MentionDecision::Allow;
    }
    if mention_present || thread_already_engaged || bot_has_replied_in_thread {
        MentionDecision::Allow
    } else {
        MentionDecision::Block
    }
}
