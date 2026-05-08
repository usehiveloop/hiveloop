use domain::MessageHandle;
use slack_morphism::prelude::*;

use super::context::SlackContext;
use crate::{GatewayError, Result};

pub async fn add(context: &SlackContext, handle: &MessageHandle, emoji: &str) -> Result<()> {
    let session = context.open_api_session();
    let request = SlackApiReactionsAddRequest::new(
        SlackChannelId(handle.channel.clone()),
        SlackReactionName(emoji.to_string()),
        SlackTs(handle.ts.clone()),
    );
    if let Err(error) = session.reactions_add(&request).await {
        if reaction_already_exists(&error) {
            return Ok(());
        }
        return Err(GatewayError::Other(anyhow::anyhow!(
            "reactions.add: {error}"
        )));
    }
    Ok(())
}

pub async fn remove(context: &SlackContext, handle: &MessageHandle, emoji: &str) -> Result<()> {
    let session = context.open_api_session();
    let request = SlackApiReactionsRemoveRequest::new(
        SlackReactionName(emoji.to_string()),
    )
    .with_channel(SlackChannelId(handle.channel.clone()))
    .with_timestamp(SlackTs(handle.ts.clone()));
    if let Err(error) = session.reactions_remove(&request).await {
        if reaction_does_not_exist(&error) {
            return Ok(());
        }
        return Err(GatewayError::Other(anyhow::anyhow!(
            "reactions.remove: {error}"
        )));
    }
    Ok(())
}

fn reaction_already_exists<E: std::fmt::Display>(error: &E) -> bool {
    error.to_string().contains("already_reacted")
}

fn reaction_does_not_exist<E: std::fmt::Display>(error: &E) -> bool {
    let rendered = error.to_string();
    rendered.contains("no_reaction") || rendered.contains("not_reacted")
}
