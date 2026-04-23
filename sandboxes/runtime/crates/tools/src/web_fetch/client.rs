//! HTTP fetching for the WebFetch tool: fallback service and reqwest-based fetch.

use std::time::Duration;

use super::parser::{extract_article, fallback_convert, strip_html_tags, truncate_content};
use super::{FetchFormat, FetchResult, WebFetchTool, MAX_RESPONSE_SIZE};

/// Build the default reqwest client with standard settings.
pub(super) fn build_default_client() -> reqwest::Client {
    reqwest::Client::builder()
        .timeout(Duration::from_secs(30))
        .redirect(reqwest::redirect::Policy::limited(10))
        .user_agent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
        .build()
        .expect("Failed to build reqwest client")
}

impl WebFetchTool {
    /// Fetch using an external fallback service.
    pub(super) async fn fetch_with_fallback(
        &self,
        url: &str,
        max_length: usize,
        format: &FetchFormat,
    ) -> Result<Option<FetchResult>, String> {
        let fallback_url = match &self.fallback_url {
            Some(u) => u,
            None => return Ok(None),
        };

        let format_str = match format {
            FetchFormat::Markdown => "markdown",
            FetchFormat::Text => "text",
            FetchFormat::Html => "html",
        };

        let resp = self
            .client
            .post(fallback_url)
            .json(&serde_json::json!({
                "url": url,
                "format": format_str,
            }))
            .timeout(Duration::from_secs(30))
            .send()
            .await
            .map_err(|e| format!("Fallback service request failed: {e}"))?;

        if !resp.status().is_success() {
            return Err(format!("Fallback service returned HTTP {}", resp.status()));
        }

        let body: serde_json::Value = resp
            .json()
            .await
            .map_err(|e| format!("Fallback service response parse failed: {e}"))?;

        let content = match body.get("content").and_then(|c| c.as_str()) {
            Some(c) if !c.trim().is_empty() => c.to_string(),
            _ => return Ok(None),
        };

        let title = body.get("title").and_then(|t| t.as_str()).map(String::from);

        Ok(Some(FetchResult {
            title,
            content: truncate_content(&content, max_length),
            url: url.to_string(),
        }))
    }

    /// Tier 2: Fetch using reqwest + readability (existing pipeline).
    /// Also used directly by unit tests that mock HTTP responses.
    pub async fn fetch_with_reqwest(
        &self,
        url: &str,
        max_length: usize,
        format: &FetchFormat,
    ) -> Result<FetchResult, String> {
        // Build Accept header based on format
        let accept_header = match format {
            FetchFormat::Markdown => "text/markdown;q=1.0, text/x-markdown;q=0.9, text/plain;q=0.8, text/html;q=0.7, */*;q=0.1",
            FetchFormat::Text => "text/plain;q=1.0, text/markdown;q=0.9, text/html;q=0.8, */*;q=0.1",
            FetchFormat::Html => "text/html;q=1.0, application/xhtml+xml;q=0.9, text/plain;q=0.8, */*;q=0.1",
        };

        // 1. HTTP GET with timeout and redirect following
        let response = self
            .client
            .get(url)
            .header("Accept", accept_header)
            .timeout(Duration::from_secs(30))
            .send()
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    format!("Request timed out: {e}")
                } else if e.is_redirect() {
                    format!("Too many redirects: {e}")
                } else {
                    format!("Request failed: {e}")
                }
            })?;

        let status = response.status();
        let final_url = response.url().to_string();

        // Check for Cloudflare challenge on 403
        let response = if status == reqwest::StatusCode::FORBIDDEN {
            let cf_mitigated = response
                .headers()
                .get("cf-mitigated")
                .and_then(|v| v.to_str().ok())
                .map(|s| s.to_string());

            if cf_mitigated.as_deref() == Some("challenge") {
                // Retry with simpler User-Agent
                let retry = self
                    .client
                    .get(url)
                    .header("User-Agent", "bridge")
                    .timeout(Duration::from_secs(30))
                    .send()
                    .await
                    .map_err(|e| format!("Cloudflare retry failed: {e}"))?;

                if retry.status().is_success() {
                    retry
                } else {
                    return Err(format!(
                        "HTTP error {} for URL: {}",
                        retry.status(),
                        final_url
                    ));
                }
            } else {
                return Err(format!("HTTP error {status} for URL: {final_url}"));
            }
        } else if !status.is_success() {
            return Err(format!("HTTP error {status} for URL: {final_url}"));
        } else {
            response
        };

        let final_url = response.url().to_string();

        // Check Content-Length header before reading body
        if let Some(content_length) = response
            .headers()
            .get(reqwest::header::CONTENT_LENGTH)
            .and_then(|v| v.to_str().ok())
            .and_then(|v| v.parse::<usize>().ok())
        {
            if content_length > MAX_RESPONSE_SIZE {
                return Err("Response too large (exceeds 5MB limit)".to_string());
            }
        }

        // Check content type
        let content_type_str = response
            .headers()
            .get(reqwest::header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok())
            .unwrap_or("")
            .to_string();

        // Handle image content types (return as base64) — except SVG
        if content_type_str.starts_with("image/")
            && content_type_str != "image/svg+xml"
            && !content_type_str.contains("vnd.fastbidsheet")
        {
            let bytes = response
                .bytes()
                .await
                .map_err(|e| format!("Failed to read image: {e}"))?;

            if bytes.len() > MAX_RESPONSE_SIZE {
                return Err("Response too large (exceeds 5MB limit)".to_string());
            }

            use base64::Engine;
            let b64 = base64::engine::general_purpose::STANDARD.encode(&bytes);

            let content = format!("data:{};base64,{}", content_type_str, b64);
            return Ok(FetchResult {
                title: Some(format!("Image ({})", content_type_str)),
                content,
                url: final_url,
            });
        }

        if !content_type_str.is_empty() {
            let is_html = content_type_str
                .parse::<mime::Mime>()
                .map(|m| {
                    (m.type_() == mime::TEXT && m.subtype() == mime::HTML)
                        || (m.type_() == mime::APPLICATION
                            && m.subtype().as_str().starts_with("xhtml"))
                        || content_type_str.starts_with("image/svg+xml")
                })
                .unwrap_or(false);

            if !is_html {
                return Err(format!(
                    "Non-HTML content type: {}. Only HTML pages are supported.",
                    content_type_str
                ));
            }
        }

        // Read body as bytes to check size
        let body_bytes = response
            .bytes()
            .await
            .map_err(|e| format!("Failed to read response body: {e}"))?;

        if body_bytes.len() > MAX_RESPONSE_SIZE {
            return Err("Response too large (exceeds 5MB limit)".to_string());
        }

        let html = String::from_utf8_lossy(&body_bytes).to_string();

        if html.trim().is_empty() {
            return Ok(FetchResult {
                title: None,
                content: String::new(),
                url: final_url,
            });
        }

        // Handle different output formats
        match format {
            FetchFormat::Html => {
                let content = truncate_content(&html, max_length);
                Ok(FetchResult {
                    title: None,
                    content,
                    url: final_url,
                })
            }
            FetchFormat::Text => {
                let text = strip_html_tags(&html);
                let content = truncate_content(&text, max_length);
                Ok(FetchResult {
                    title: None,
                    content,
                    url: final_url,
                })
            }
            FetchFormat::Markdown => {
                // 2. Try dom_smoothie readability extraction first
                if let Some(article) = extract_article(&html, &final_url) {
                    let title = if article.title.is_empty() {
                        None
                    } else {
                        Some(article.title)
                    };
                    let content = truncate_content(&article.text_content, max_length);
                    return Ok(FetchResult {
                        title,
                        content,
                        url: final_url,
                    });
                }

                // 3. Fallback: convert full HTML to markdown with htmd
                let markdown = fallback_convert(&html);
                let content = truncate_content(&markdown, max_length);

                Ok(FetchResult {
                    title: None,
                    content,
                    url: final_url,
                })
            }
        }
    }
}
