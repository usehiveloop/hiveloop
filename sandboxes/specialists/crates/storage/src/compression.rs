use std::io;

/// zstd compression level — 3 is a good balance of speed vs. ratio.
const COMPRESSION_LEVEL: i32 = 3;

/// Payloads smaller than this are stored uncompressed.
const COMPRESSION_THRESHOLD: usize = 256;

/// Prefix byte: data is stored uncompressed.
const PREFIX_RAW: u8 = 0x00;
/// Prefix byte: data is zstd-compressed.
const PREFIX_ZSTD: u8 = 0x01;

/// Compress `data`, prepending a one-byte flag.
///
/// Small payloads (< 256 bytes) are stored with a `0x00` prefix (raw).
/// Larger payloads get a `0x01` prefix followed by zstd-compressed bytes.
pub fn compress(data: &[u8]) -> io::Result<Vec<u8>> {
    if data.len() < COMPRESSION_THRESHOLD {
        let mut out = Vec::with_capacity(1 + data.len());
        out.push(PREFIX_RAW);
        out.extend_from_slice(data);
        return Ok(out);
    }
    let compressed = zstd::encode_all(data, COMPRESSION_LEVEL)?;
    let mut out = Vec::with_capacity(1 + compressed.len());
    out.push(PREFIX_ZSTD);
    out.extend_from_slice(&compressed);
    Ok(out)
}

/// Decompress a blob produced by [`compress`].
pub fn decompress(data: &[u8]) -> io::Result<Vec<u8>> {
    if data.is_empty() {
        return Ok(Vec::new());
    }
    match data[0] {
        PREFIX_RAW => Ok(data[1..].to_vec()),
        PREFIX_ZSTD => zstd::decode_all(&data[1..]),
        other => Err(io::Error::new(
            io::ErrorKind::InvalidData,
            format!("unknown compression prefix: 0x{other:02x}"),
        )),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn small_payload_stored_raw() {
        let input = b"hello";
        let blob = compress(input).unwrap();
        assert_eq!(blob[0], PREFIX_RAW);
        assert_eq!(&blob[1..], input);
        assert_eq!(decompress(&blob).unwrap(), input);
    }

    #[test]
    fn large_payload_compressed() {
        let input = vec![0x42; 1024];
        let blob = compress(&input).unwrap();
        assert_eq!(blob[0], PREFIX_ZSTD);
        assert!(blob.len() < input.len()); // zstd should compress repeated bytes well
        assert_eq!(decompress(&blob).unwrap(), input);
    }

    #[test]
    fn roundtrip_json() {
        let json = serde_json::json!({
            "key": "value",
            "data": "x".repeat(500),
        });
        let bytes = serde_json::to_vec(&json).unwrap();
        let blob = compress(&bytes).unwrap();
        let restored = decompress(&blob).unwrap();
        assert_eq!(bytes, restored);
    }

    #[test]
    fn empty_input() {
        let blob = compress(b"").unwrap();
        assert_eq!(blob, &[PREFIX_RAW]);
        assert_eq!(decompress(&blob).unwrap(), b"");
    }

    #[test]
    fn decompress_empty_slice() {
        assert_eq!(decompress(&[]).unwrap(), b"");
    }

    #[test]
    fn decompress_unknown_prefix() {
        let err = decompress(&[0xFF, 0x01]).unwrap_err();
        assert_eq!(err.kind(), io::ErrorKind::InvalidData);
    }
}
