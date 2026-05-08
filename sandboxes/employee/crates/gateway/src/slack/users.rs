use chrono::Utc;
use slack_morphism::prelude::*;

use super::context::{CachedDisplayName, SlackContext};
use crate::{GatewayError, Result};

const DISPLAY_NAME_TTL_SECONDS: i64 = 60 * 60;

pub async fn resolve_display_name(context: &SlackContext, user_id: &str) -> Result<String> {
    if let Some(cached) = context.user_display_name_cache.get(user_id) {
        let age = Utc::now() - cached.fetched_at;
        if age.num_seconds() < DISPLAY_NAME_TTL_SECONDS {
            return Ok(cached.name.clone());
        }
    }
    let fresh = fetch_display_name_from_api(context, user_id).await?;
    context.user_display_name_cache.insert(
        user_id.to_string(),
        CachedDisplayName {
            name: fresh.clone(),
            fetched_at: Utc::now(),
        },
    );
    Ok(fresh)
}

async fn fetch_display_name_from_api(context: &SlackContext, user_id: &str) -> Result<String> {
    let session = context.open_api_session();
    let request = SlackApiUsersInfoRequest::new(SlackUserId(user_id.to_string()));
    let response = session
        .users_info(&request)
        .await
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("users.info: {e}")))?;
    let profile = &response.user;
    let real_name = profile.real_name.clone();
    let name = profile.name.clone();
    Ok(real_name
        .or(name)
        .unwrap_or_else(|| user_id.to_string()))
}
