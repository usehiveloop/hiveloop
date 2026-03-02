use base64::Engine;
use hmac::{Hmac, Mac};
use sha2::Sha256;

/// Sign a webhook payload with HMAC-SHA256.
/// Message format: "{timestamp}.{payload}"
/// Returns base64-encoded signature.
pub fn sign_webhook(payload: &[u8], secret: &str, timestamp: i64) -> String {
    let payload_str = std::str::from_utf8(payload).unwrap_or("");
    let message = format!("{}.{}", timestamp, payload_str);
    let mut mac =
        Hmac::<Sha256>::new_from_slice(secret.as_bytes()).expect("HMAC can take key of any size");
    mac.update(message.as_bytes());
    let result = mac.finalize();
    base64::engine::general_purpose::STANDARD.encode(result.into_bytes())
}

/// Verify a webhook signature using constant-time comparison.
pub fn verify_webhook(payload: &[u8], secret: &str, timestamp: i64, signature: &str) -> bool {
    let expected = sign_webhook(payload, secret, timestamp);
    constant_time_eq(expected.as_bytes(), signature.as_bytes())
}

fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    a.iter()
        .zip(b.iter())
        .fold(0u8, |acc, (x, y)| acc | (x ^ y))
        == 0
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_sign_webhook_deterministic() {
        let payload = b"hello world";
        let secret = "my-secret";
        let timestamp = 1700000000;

        let sig1 = sign_webhook(payload, secret, timestamp);
        let sig2 = sign_webhook(payload, secret, timestamp);

        assert_eq!(
            sig1, sig2,
            "sign_webhook should produce deterministic output"
        );
        assert!(!sig1.is_empty(), "signature should not be empty");
    }

    #[test]
    fn test_verify_webhook_valid_signature() {
        let payload = b"test payload";
        let secret = "webhook-secret";
        let timestamp = 1700000000;

        let signature = sign_webhook(payload, secret, timestamp);
        assert!(
            verify_webhook(payload, secret, timestamp, &signature),
            "verify_webhook should return true for a valid signature"
        );
    }

    #[test]
    fn test_verify_webhook_tampered_payload() {
        let payload = b"original payload";
        let secret = "webhook-secret";
        let timestamp = 1700000000;

        let signature = sign_webhook(payload, secret, timestamp);
        let tampered = b"tampered payload";
        assert!(
            !verify_webhook(tampered, secret, timestamp, &signature),
            "verify_webhook should return false for a tampered payload"
        );
    }

    #[test]
    fn test_verify_webhook_wrong_secret() {
        let payload = b"test payload";
        let secret = "correct-secret";
        let wrong_secret = "wrong-secret";
        let timestamp = 1700000000;

        let signature = sign_webhook(payload, secret, timestamp);
        assert!(
            !verify_webhook(payload, wrong_secret, timestamp, &signature),
            "verify_webhook should return false for a wrong secret"
        );
    }
}
