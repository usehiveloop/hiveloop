use domain::{Attachment, SlackConfig};
use gateway::ChannelGateway;
use tracing::{info, warn};

const MAX_IMAGE_BYTES: usize = 10 * 1024 * 1024;

pub struct DownloadResults {
    pub images: Vec<(String, Vec<u8>)>,
    pub text_files: Vec<InlinedTextFile>,
    pub audio_summaries: Vec<String>,
    pub document_summaries: Vec<String>,
    pub failure_notices: Vec<String>,
}

pub struct InlinedTextFile {
    pub name: String,
    pub contents: String,
}

pub async fn collect_media_for_turn(
    gateway: &dyn ChannelGateway,
    attachments: &[Attachment],
    slack: &SlackConfig,
    multimodal_available: bool,
) -> DownloadResults {
    let mut images = Vec::new();
    let mut text_files = Vec::new();
    let mut audio_summaries = Vec::new();
    let mut document_summaries = Vec::new();
    let mut failure_notices = Vec::new();

    for attachment in attachments {
        if attachment.mime_type.starts_with("image/") {
            if multimodal_available {
                handle_image_download(gateway, attachment, &mut images, &mut failure_notices).await;
            } else {
                tracing::info!(
                    name = %attachment.name,
                    "no multimodal model configured; skipping image download (annotation only)"
                );
            }
        } else if attachment.mime_type.starts_with("audio/") {
            handle_audio(
                gateway,
                attachment,
                &mut audio_summaries,
                &mut failure_notices,
            )
            .await;
        } else if is_text_inlinable_mime(&attachment.mime_type)
            || mime_is_textual_extension(&attachment.name)
        {
            handle_text_file_download(
                gateway,
                attachment,
                slack.inline_text_files,
                slack.inline_text_max_bytes,
                &mut text_files,
                &mut document_summaries,
                &mut failure_notices,
            )
            .await;
        } else {
            document_summaries.push(format_document_summary(attachment));
        }
    }

    DownloadResults {
        images,
        text_files,
        audio_summaries,
        document_summaries,
        failure_notices,
    }
}

async fn handle_image_download(
    gateway: &dyn ChannelGateway,
    attachment: &Attachment,
    images: &mut Vec<(String, Vec<u8>)>,
    failure_notices: &mut Vec<String>,
) {
    match gateway.download_attachment(attachment).await {
        Ok(bytes) if bytes.len() <= MAX_IMAGE_BYTES => {
            info!(name = %attachment.name, size = bytes.len(), "downloaded image for vision");
            images.push((attachment.mime_type.clone(), bytes));
        }
        Ok(bytes) => {
            warn!(name = %attachment.name, size = bytes.len(), limit = MAX_IMAGE_BYTES, "image too large");
            failure_notices.push(format!(
                "{} skipped: image exceeds {} byte limit",
                attachment.name, MAX_IMAGE_BYTES
            ));
        }
        Err(e) => {
            warn!(name = %attachment.name, error = %e, "image download failed");
            failure_notices.push(format!("{} could not be downloaded ({e})", attachment.name));
        }
    }
}

async fn handle_audio(
    gateway: &dyn ChannelGateway,
    attachment: &Attachment,
    audio_summaries: &mut Vec<String>,
    failure_notices: &mut Vec<String>,
) {
    match gateway.download_attachment(attachment).await {
        Ok(bytes) => {
            audio_summaries.push(format!(
                "{} ({}, {} bytes) — audio cannot be transcribed by the current model",
                attachment.name,
                attachment.mime_type,
                bytes.len()
            ));
        }
        Err(e) => {
            warn!(name = %attachment.name, error = %e, "audio download failed");
            failure_notices.push(format!(
                "{} (audio) could not be downloaded ({e})",
                attachment.name
            ));
        }
    }
}

async fn handle_text_file_download(
    gateway: &dyn ChannelGateway,
    attachment: &Attachment,
    inlining_enabled: bool,
    max_bytes: u64,
    text_files: &mut Vec<InlinedTextFile>,
    document_summaries: &mut Vec<String>,
    failure_notices: &mut Vec<String>,
) {
    if !inlining_enabled {
        document_summaries.push(format_document_summary(attachment));
        return;
    }
    match gateway.download_attachment(attachment).await {
        Ok(bytes) if (bytes.len() as u64) <= max_bytes => match String::from_utf8(bytes) {
            Ok(text) => {
                info!(name = %attachment.name, chars = text.len(), "inlined text file");
                text_files.push(InlinedTextFile {
                    name: attachment.name.clone(),
                    contents: text,
                });
            }
            Err(_) => {
                warn!(name = %attachment.name, "non-utf8 text file; treating as document");
                document_summaries.push(format_document_summary(attachment));
            }
        },
        Ok(bytes) => {
            warn!(name = %attachment.name, size = bytes.len(), limit = max_bytes, "text file exceeds inline limit");
            document_summaries.push(format_document_summary(attachment));
        }
        Err(e) => {
            warn!(name = %attachment.name, error = %e, "text file download failed");
            failure_notices.push(format!("{} could not be downloaded ({e})", attachment.name));
        }
    }
}

fn is_text_inlinable_mime(mime: &str) -> bool {
    mime.starts_with("text/")
        || matches!(
            mime,
            "application/json"
                | "application/yaml"
                | "application/x-yaml"
                | "application/xml"
                | "application/toml"
                | "application/x-toml"
                | "application/javascript"
                | "application/x-sh"
        )
}

fn mime_is_textual_extension(name: &str) -> bool {
    let lower = name.to_lowercase();
    [
        ".md",
        ".txt",
        ".csv",
        ".json",
        ".yaml",
        ".yml",
        ".toml",
        ".xml",
        ".ini",
        ".cfg",
        ".log",
        ".rst",
        ".sql",
        ".py",
        ".rs",
        ".ts",
        ".js",
        ".tsx",
        ".jsx",
        ".html",
        ".css",
        ".sh",
        ".bash",
        ".zsh",
        ".fish",
        ".env",
        ".dockerfile",
    ]
    .iter()
    .any(|extension| lower.ends_with(extension))
}

fn format_document_summary(attachment: &Attachment) -> String {
    format!("{} ({})", attachment.name, attachment.mime_type)
}
