use std::time::Duration;

use domain::{Attachment, MessageHandle, SessionId};
use reqwest::Client as HttpClient;
use slack_morphism::prelude::*;

use super::context::SlackContext;
use super::retry::with_retry;
use super::session_keys::{message_handle, split_session_id};
use crate::{GatewayError, Result};

const HTTP_TIMEOUT_SECONDS: u64 = 30;
const HTML_CONTENT_TYPE_PREFIX: &str = "text/html";
const DEFAULT_RETRY_ATTEMPTS: u32 = 3;

pub async fn download_attachment(
    context: &SlackContext,
    attachment: &Attachment,
) -> Result<Vec<u8>> {
    let bot_token = context.bot_token.token_value.0.clone();
    let max_attempts = context
        .config
        .snapshot()
        .slack
        .retry_max_attempts
        .unwrap_or(DEFAULT_RETRY_ATTEMPTS);
    let url = attachment.url.clone();
    with_retry(max_attempts, || async {
        run_single_download(&bot_token, &url).await
    })
    .await
}

async fn run_single_download(bot_token: &str, url: &str) -> Result<Vec<u8>> {
    let http = build_http_client()?;
    let response = http
        .get(url)
        .bearer_auth(bot_token)
        .send()
        .await
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("attachment fetch: {e}")))?;
    let response = response
        .error_for_status()
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("attachment status: {e}")))?;
    let content_type = response
        .headers()
        .get(reqwest::header::CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
        .to_string();
    if content_type.starts_with(HTML_CONTENT_TYPE_PREFIX) {
        return Err(GatewayError::Unauthorized(
            "Slack returned HTML; bot token likely lacks file access".into(),
        ));
    }
    let body_bytes = response
        .bytes()
        .await
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("attachment bytes: {e}")))?;
    Ok(body_bytes.to_vec())
}

pub async fn upload_file(
    context: &SlackContext,
    session_id: &SessionId,
    bytes: Vec<u8>,
    filename: &str,
    caption: Option<&str>,
) -> Result<MessageHandle> {
    let (channel, thread_ts) = split_session_id(session_id)?;
    let upload_target = request_upload_url(context, filename, bytes.len()).await?;

    let http = build_http_client()?;
    http.post(&upload_target.upload_url)
        .body(bytes)
        .send()
        .await
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("upload PUT: {e}")))?
        .error_for_status()
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("upload status: {e}")))?;

    finalize_upload(
        context,
        &upload_target.file_id,
        filename,
        &channel,
        &thread_ts,
        caption,
    )
    .await
}

struct UploadTarget {
    upload_url: String,
    file_id: String,
}

async fn request_upload_url(
    context: &SlackContext,
    filename: &str,
    length: usize,
) -> Result<UploadTarget> {
    let bot_token = context.bot_token.token_value.0.clone();
    let http = build_http_client()?;
    let response: serde_json::Value = http
        .post("https://slack.com/api/files.getUploadURLExternal")
        .bearer_auth(&bot_token)
        .form(&[
            ("filename", filename.to_string()),
            ("length", length.to_string()),
        ])
        .send()
        .await
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("getUploadURLExternal: {e}")))?
        .json()
        .await
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("getUploadURLExternal json: {e}")))?;
    if !response
        .get("ok")
        .and_then(|v| v.as_bool())
        .unwrap_or(false)
    {
        return Err(GatewayError::Other(anyhow::anyhow!(
            "getUploadURLExternal failed: {response}"
        )));
    }
    Ok(UploadTarget {
        upload_url: response
            .get("upload_url")
            .and_then(|v| v.as_str())
            .ok_or_else(|| GatewayError::Other(anyhow::anyhow!("missing upload_url")))?
            .to_string(),
        file_id: response
            .get("file_id")
            .and_then(|v| v.as_str())
            .ok_or_else(|| GatewayError::Other(anyhow::anyhow!("missing file_id")))?
            .to_string(),
    })
}

async fn finalize_upload(
    context: &SlackContext,
    file_id: &str,
    filename: &str,
    channel: &SlackChannelId,
    thread_ts: &SlackTs,
    caption: Option<&str>,
) -> Result<MessageHandle> {
    let bot_token = context.bot_token.token_value.0.clone();
    let http = build_http_client()?;
    let files_array = serde_json::json!([{ "id": file_id, "title": filename }]);
    let mut form_pairs: Vec<(&str, String)> = vec![
        ("channel_id", channel.0.clone()),
        ("files", files_array.to_string()),
        ("thread_ts", thread_ts.0.clone()),
    ];
    if let Some(caption_text) = caption {
        form_pairs.push(("initial_comment", caption_text.to_string()));
    }
    let response: serde_json::Value = http
        .post("https://slack.com/api/files.completeUploadExternal")
        .bearer_auth(&bot_token)
        .form(&form_pairs)
        .send()
        .await
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("completeUploadExternal: {e}")))?
        .json()
        .await
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("completeUploadExternal json: {e}")))?;
    if !response
        .get("ok")
        .and_then(|v| v.as_bool())
        .unwrap_or(false)
    {
        return Err(GatewayError::Other(anyhow::anyhow!(
            "completeUploadExternal failed: {response}"
        )));
    }
    Ok(message_handle(&channel.0, &thread_ts.0))
}

fn build_http_client() -> Result<HttpClient> {
    HttpClient::builder()
        .timeout(Duration::from_secs(HTTP_TIMEOUT_SECONDS))
        .build()
        .map_err(|e| GatewayError::Other(anyhow::anyhow!("http client: {e}")))
}
