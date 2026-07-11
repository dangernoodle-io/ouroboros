package embed

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	require.Equal(t, 512, Dim())
	require.NotNil(t, loadedAsset)
	require.Equal(t, loadedAsset.vocab, len(vocabTxtLines(t)))
}

func vocabTxtLines(t *testing.T) []string {
	t.Helper()
	vocab, _, err := loadVocab(vocabTxt)
	require.NoError(t, err)
	lines := make([]string, len(vocab))
	for tok, id := range vocab {
		lines[id] = tok
	}
	return lines
}

type tokenizerFixture struct {
	Text string  `json:"text"`
	IDs  []int32 `json:"ids"`
}

func TestTokenizerParity(t *testing.T) {
	raw, err := os.ReadFile("testdata/tokenizer_parity.json")
	require.NoError(t, err)

	var fixtures []tokenizerFixture
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	require.NotEmpty(t, fixtures)

	for _, fx := range fixtures {
		got := tokenizer.tokenize(fx.Text)
		if len(fx.IDs) == 0 && len(got) == 0 {
			continue // both empty; require.Equal on nil vs []int32{} is fine too but be explicit
		}
		require.Equal(t, fx.IDs, got, "tokenize(%q)", fx.Text)
	}
}

type embeddingFixture struct {
	Text      string    `json:"text"`
	Embedding []float32 `json:"embedding"`
}

// TestEmbeddingParity is THE gate: Go Embed() must match a Python
// reference that applies the SAME int4-per-row quantization scheme
// (see gen/generate_golden.py) with cosine >= 0.999 for every sample.
// This proves the Go tokenize/gather/pool/normalize/dequant pipeline
// matches the reference int4 scheme bit-for-bit-ish (only float
// accumulation order differs). It does NOT claim int4 matches fp32 --
// that tradeoff (~0.98 cosine, recall@5 ~0.85 vs fp32) is a separate,
// spike-approved quality decision documented in README.md.
func TestEmbeddingParity(t *testing.T) {
	const cosineFloor = 0.999

	raw, err := os.ReadFile("testdata/embedding_golden.json")
	require.NoError(t, err)

	var fixtures []embeddingFixture
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	require.NotEmpty(t, fixtures)

	minCos := float32(1)
	nonEmpty := 0
	for _, fx := range fixtures {
		got := Embed(fx.Text)
		require.Len(t, got, len(fx.Embedding), "dim mismatch for %q", fx.Text)

		if fx.Text == "" {
			// Empty input: both sides are the zero vector; cosine is
			// undefined (0/0), not a meaningful parity check.
			require.Equal(t, make([]float32, Dim()), got)
			continue
		}

		cos := Cosine(got, fx.Embedding)
		nonEmpty++
		if cos < minCos {
			minCos = cos
		}
		require.GreaterOrEqualf(t, cos, float32(cosineFloor), "cosine parity for %q: got %.6f", fx.Text, cos)
	}

	t.Logf("embedding parity: %d samples, min cosine=%.6f (floor=%.3f)", nonEmpty, minCos, cosineFloor)
}

// TestQualitySanity reproduces a couple of the spike's relevance
// wins: a semantically related pair should score clearly higher than
// an unrelated pair.
func TestQualitySanity(t *testing.T) {
	authQuery := Embed("auth approach")
	related := Embed("API bearer token authentication")
	unrelated := Embed("completely unrelated topic about gardening")

	relatedScore := Cosine(authQuery, related)
	unrelatedScore := Cosine(authQuery, unrelated)
	require.Greater(t, relatedScore, unrelatedScore,
		"expected %q to score higher than %q against %q", "API bearer token authentication",
		"completely unrelated topic about gardening", "auth approach")
	require.Greater(t, relatedScore, float32(0.3))

	storageQuery := Embed("how do we store data")
	storageRelated := Embed("semantic search implementation")
	storageUnrelated := Embed("wifi reconnect stability issues")

	require.Greater(t, Cosine(storageQuery, storageRelated), Cosine(storageQuery, storageUnrelated))
}

func TestEmbedEmptyString(t *testing.T) {
	got := Embed("")
	want := make([]float32, Dim())
	require.Equal(t, want, got)
}

func TestEmbedAllUnknownTokens(t *testing.T) {
	// A CJK-only string: every character is an isolated, out-of-vocab
	// token for this English-centric vocabulary, so every token is
	// [UNK] and gets dropped -> zero vector, matching model2vec.
	got := Embed("汉字测试")
	want := make([]float32, Dim())
	require.Equal(t, want, got)
}

func TestCosineMismatchedLength(t *testing.T) {
	require.Equal(t, float32(0), Cosine([]float32{1, 2}, []float32{1, 2, 3}))
	require.Equal(t, float32(0), Cosine(nil, nil))
}

func TestCosineZeroVector(t *testing.T) {
	zero := make([]float32, Dim())
	other := Embed("some text")
	require.Equal(t, float32(0), Cosine(zero, other))
}
