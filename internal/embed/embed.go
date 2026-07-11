// Package embed provides pure-Go, offline static text embeddings for
// semantic search over the ouroboros knowledge base and backlog.
//
// Model: Model2Vec "minishlab/potion-retrieval-32M" (MIT license,
// https://huggingface.co/minishlab/potion-retrieval-32M), a
// distilled, retrieval-tuned static (non-contextual) embedding model:
// a [vocab x 512] float32 token embedding table plus a standard BERT
// WordPiece tokenizer (base model: baai/bge-base-en-v1.5). Embedding a
// string is tokenize -> gather per-token rows -> mean-pool ->
// L2-normalize; no transformer forward pass, no CGO, no network call.
//
// Quantization: the embedding matrix is quantized to int4 with a
// per-row (per-token) symmetric scale, packed two values per byte (see
// internal/embed/gen), which shrinks the ~123MB fp32 matrix to ~16MB.
// The golden parity gate (embed_test.go) compares the Go int4 Embed()
// against a Python reference that applies the SAME int4 quantization
// (cosine >= 0.999, validating the Go implementation against the
// scheme rather than against unquantized fp32). Separately, int4 vs
// the original fp32 model loses some quality (~0.98 cosine on
// representative queries, recall@5 ~0.85 vs fp32 top-5, spike-approved
// tradeoff -- see internal/embed/README.md). Regenerate the asset with
// internal/embed/gen/generate.py; regenerate the test fixtures with
// internal/embed/gen/generate_golden.py. Both scripts need network
// access (or a warm huggingface_hub cache); the resulting artifacts
// are committed and //go:embed'd, so package tests are hermetic.
//
// Asset format (internal/embed/data/model.bin) is documented in
// internal/embed/gen/generate.py and internal/embed/README.md.
package embed

import "math"

// ModelID stamps embeddings produced by this package, so stored
// vectors can be tied to the model + quantization scheme that produced
// them (and invalidated if either ever changes).
const ModelID = "potion-retrieval-32M-int4row-v1"

var (
	loadedAsset *asset
	tokenizer   *wordpiece
)

func init() {
	// Panicking here is deliberate: the asset is //go:embed'd at
	// compile time, so a load failure means the embedded model.bin or
	// vocab.txt is corrupt or stale relative to this code — a
	// build/regen defect, not a runtime condition. Fail loud rather
	// than silently degrading. Wiring this package into the MCP
	// server will need its own deliberate degrade-vs-crash decision
	// (e.g. disabling semantic search vs. failing server startup)
	// rather than inheriting this panic as-is.
	a, err := loadAsset(modelBin)
	if err != nil {
		panic("embed: failed to load embedded model asset: " + err.Error())
	}
	vocab, unkID, err := loadVocab(vocabTxt)
	if err != nil {
		panic("embed: failed to load embedded vocab: " + err.Error())
	}
	if len(vocab) != a.vocab {
		panic("embed: vocab size mismatch between model.bin and vocab.txt")
	}
	loadedAsset = a
	tokenizer = newWordpiece(vocab, unkID)
}

// Dim returns the embedding dimensionality (512 for potion-retrieval-32M).
func Dim() int {
	return loadedAsset.dim
}

// Embed returns the L2-normalized static embedding of text: normalize
// and tokenize, gather each token's embedding row, mean-pool, then
// L2-normalize. Mirrors model2vec.StaticModel.encode exactly (no
// special tokens added; [UNK] tokens are dropped, not embedded).
// A string that has no embeddable tokens (empty, or every token
// unknown) returns a zero vector, matching model2vec's behavior.
func Embed(text string) []float32 {
	ids := tokenizer.tokenize(text)
	dim := loadedAsset.dim
	sum := make([]float32, dim)
	if len(ids) == 0 {
		return sum
	}

	for _, id := range ids {
		loadedAsset.accumulateRow(id, sum)
	}

	inv := 1.0 / float32(len(ids))
	for i := range sum {
		sum[i] *= inv
	}

	norm := float32(0)
	for _, v := range sum {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm))) + 1e-32
	for i := range sum {
		sum[i] /= norm
	}

	return sum
}

// Cosine returns the cosine similarity between two equal-length
// embeddings. Returns 0 if either vector has zero norm or the lengths
// differ.
func Cosine(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
