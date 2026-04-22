// Package filestore stores raw connector payloads, chunked-doc artifacts,
// and indexing checkpoints in S3-compatible object storage.
//
// Ports backend/onyx/file_store/ — the FileStore protocol and its S3
// implementation. In Hiveloop we reuse the same R2/MinIO bucket that backs
// the LanceDB dataset, prefix-isolated.
package filestore
