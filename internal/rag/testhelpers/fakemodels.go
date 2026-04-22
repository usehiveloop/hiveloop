package testhelpers

// Go-side companion for the Rust `FakeEmbedder` living at
// `services/rag-engine/crates/rag-engine-embed/src/fake.rs`.
//
// The Rust implementation is the source of truth; this file mirrors the
// exact algorithm so Go tests can precompute the vector the Rust engine
// will produce for a given (text, kind) pair. That lets tests assert
// top-K ordering without round-tripping through the service when all
// they want to verify is fusion / scoring math.
//
// Algorithm (MUST stay byte-identical with fake.rs):
//
//	1. kind_byte = 0x00 for Passage, 0x01 for Query
//	2. digest = SHA-256(kind_byte || utf8(text))
//	3. seed ChaCha20 with the full 32-byte digest
//	4. draw `dim` float32s uniform in [-1, 1)
//	5. L2-normalize
//
// The only subtle bit is the uniform-f32 draw. rand-chacha with rand
// crate's `random_range(-1.0..1.0)` uses the `StandardUniform` sampler,
// which for f32 produces `u32_sample >> 8` cast to f32 in [0, 1) and
// then scales. We reproduce that by taking 4 bytes at a time from the
// keystream and applying the same 24-bit mantissa mapping.

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"golang.org/x/crypto/chacha20"
)

// FakeEmbedKind mirrors the Rust `EmbedKind` enum. Passage is the kind
// used for ingested chunks; Query is used for search requests.
type FakeEmbedKind int

const (
	FakeEmbedKindPassage FakeEmbedKind = 0
	FakeEmbedKindQuery   FakeEmbedKind = 1
)

// FakeVector returns the deterministic L2-normalized float32 vector
// that the Rust `FakeEmbedder` produces for the same inputs.
//
// This is a pure function: same inputs → identical output bytes across
// invocations and processes. Byte-parity with fake.rs is a correctness
// contract, not an implementation detail — tests rely on it.
func FakeVector(text string, kind FakeEmbedKind, dim uint32) []float32 {
	// 1. kind-byte domain separation.
	var kindByte byte
	switch kind {
	case FakeEmbedKindQuery:
		kindByte = 0x01
	default:
		kindByte = 0x00
	}

	// 2. SHA-256 over (kind_byte || text).
	h := sha256.New()
	_, _ = h.Write([]byte{kindByte})
	_, _ = h.Write([]byte(text))
	digest := h.Sum(nil)

	// 3. ChaCha20Rng seeded from the 32-byte digest.
	//
	// rand-chacha's ChaCha20Rng default construction uses a 12-byte
	// all-zero nonce and a 64-stream-position counter of 0 (see
	// rand_chacha's `ChaCha20Core::from_seed`).
	var seed [32]byte
	copy(seed[:], digest)
	var nonce [12]byte
	cipher, err := chacha20.NewUnauthenticatedCipher(seed[:], nonce[:])
	if err != nil {
		// SHA-256 always produces 32 bytes; nonce is fixed-size. A
		// failure here means the crypto lib broke an invariant.
		panic("chacha20.NewUnauthenticatedCipher: " + err.Error())
	}

	// 4. Draw `dim` f32s uniform in [-1, 1). rand 0.9 with f32 uniform
	// samples via `sample_single` which reads 4 keystream bytes as a
	// u32, takes the top 24 bits as the mantissa of a float in [0, 1),
	// then maps to the target range.
	v := make([]float32, dim)
	// ChaCha20 outputs keystream when XOR'd with zero bytes.
	buf := make([]byte, 4*dim)
	cipher.XORKeyStream(buf, buf)

	const twoToThe24 = 1 << 24 // 16777216
	for i := uint32(0); i < dim; i++ {
		u := binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
		// Convert to f32 in [0, 1) using the top 24 bits as an
		// integer mantissa and dividing by 2^24.
		frac := float32(u>>8) / float32(twoToThe24)
		// Map to [-1, 1).
		v[i] = frac*2.0 - 1.0
	}

	// 5. L2 normalize. The real embedder always yields non-zero
	// vectors for non-empty text; we still guard the edge case.
	var sumSq float64
	for _, x := range v {
		sumSq += float64(x) * float64(x)
	}
	norm := float32(math.Sqrt(sumSq))
	if norm > 0 {
		for i := range v {
			v[i] /= norm
		}
	}
	return v
}

// CosineSimilarity returns the cosine similarity of two equal-length
// float32 vectors. Both inputs are expected to be unit-normalized (the
// dot product suffices) but we fall back to a full calculation when
// either norm is non-unit to be safe.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	denom := math.Sqrt(na) * math.Sqrt(nb)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
