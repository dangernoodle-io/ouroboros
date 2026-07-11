# internal/embed

Pure-Go, offline static text embeddings for future semantic search over
the ouroboros knowledge base and backlog. No CGO, no external API, no
network call at query time -- the model asset is committed and
`//go:embed`'d.

## Model

[`minishlab/potion-retrieval-32M`](https://huggingface.co/minishlab/potion-retrieval-32M)
(MIT license), a [Model2Vec](https://github.com/MinishLab/model2vec)
distilled static embedding model tuned for retrieval: a `[63091, 512]`
float32 token embedding table (base tokenizer: `baai/bge-base-en-v1.5`,
standard BERT WordPiece, 63,091 tokens). A StaticModel has no
transformer forward pass -- embedding a string is:

```
normalize + tokenize -> gather per-token rows -> mean-pool -> L2-normalize
```

This mirrors `model2vec.StaticModel.encode` exactly: no `[CLS]`/`[SEP]`
special tokens are added, and `[UNK]` tokens are dropped (not
embedded), matching model2vec's tokenizer behavior for this model.

**License attribution**: potion-retrieval-32M is MIT-licensed by
MinishLab. This package's generator scripts read the model's public
Hugging Face release; no model weights are modified beyond the int4
quantization described below.

## Quantization

The embedding matrix is quantized to int4 with a per-row (per-token)
symmetric scale, packed two values per byte: `scale[i] = max(abs(row[i]))
/ 7`, `data[i][j] = round(row[i][j] / scale[i])` clamped to `[-7,7]`.
This shrinks the ~123MB fp32 matrix to ~16MB (half of the prior int8
asset).

**int4-vs-fp32 quality tradeoff (accepted, spike-validated)**: unlike
int8 (cosine >= 0.999 vs fp32), int4 does lose meaningful precision --
observed ~0.98 cosine similarity vs the fp32 reference on
representative embeddings, and recall@5 ~0.85 vs fp32's own top-5 on a
set of concept queries (retrieval ranking is preserved; the recall gap
is mostly duplicate/near-duplicate documents swapping order within the
top-5, not unrelated results surfacing). This was validated in an
OU-68 spike before adopting int4 here, trading precision for halving
the `//go:embed`'d asset. `embed_test.go`'s golden parity gate does
NOT re-check this fp32 comparison at test time (see below); it checks
the Go implementation against an int4 reference instead.

## Asset format (`data/model.bin`)

All integers little-endian.

| Field | Type | Description |
|---|---|---|
| magic | `[4]byte` | `"M2V1"` |
| version | `uint32` | `1` |
| vocab | `uint32` | number of rows (tokens) |
| dim | `uint32` | embedding dimension (512) |
| quantScheme | `uint32` | `2` = int4 per-row symmetric, packed 2/byte (`0` = prior int8 per-row scheme, no longer emitted) |
| data | `[vocab*dim/2]byte` | row-major int4-packed matrix, 2 values/byte |
| scales | `[vocab]float32` | per-row dequantization scale |

Dequantization: unpack the 4-bit signed nibble for column `j` of row
`i` (low nibble = even `j`, high nibble = odd `j`, sign-extended from
4 bits), then `value = nibble * scales[i]`.

`data/vocab.txt` is the WordPiece vocabulary, one token per line,
ordered by token id (line 0 = token id 0, matching `data/model.bin`'s
row order).

## Regenerating the asset

Requires network access (or a warm `huggingface_hub` cache); the
generator is not run at build or test time.

```bash
pip install huggingface_hub safetensors numpy
python3 internal/embed/gen/generate.py
```

## Regenerating the test fixtures

`embed_test.go`'s parity tests load committed golden fixtures rather
than calling out to Python at test time. Regenerate them (also needs
network / a warm cache) with:

```bash
pip install model2vec tokenizers huggingface_hub
python3 internal/embed/gen/generate_golden.py
```

- `testdata/tokenizer_parity.json` -- sample strings and their reference
  WordPiece token ids (post `[UNK]`-filtering, matching
  `model2vec.StaticModel.tokenize`).
- `testdata/embedding_golden.json` -- sample strings and their reference
  embeddings, produced by applying the SAME int4-per-row quantization
  as `generate.py` to the matrix in Python, then encoding
  (tokenize -> gather -> mean-pool -> L2-normalize). This is an int4
  reference, not the original fp32 model -- see "int4-vs-fp32 quality
  tradeoff" above for that comparison.

## API

- `Embed(text string) []float32` -- 512-dim L2-normalized embedding;
  empty or all-unknown-token input returns a zero vector.
- `Cosine(a, b []float32) float32` -- cosine similarity; 0 for
  mismatched lengths or a zero vector.
- `Dim() int` -- embedding dimensionality (512).
- `ModelID` -- stamps embeddings produced by this package/quantization
  scheme, for tagging stored vectors later.
