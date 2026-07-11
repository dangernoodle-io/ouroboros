#!/usr/bin/env python3
"""Generate golden test fixtures for internal/embed's parity tests.

Produces two JSON fixtures under internal/embed/testdata/, using the
reference Python implementations (tokenizers + model2vec) so the Go
package's own tokenizer and embedder can be checked against them:

  tokenizer_parity.json  -- sample strings -> reference WordPiece token ids
  embedding_golden.json  -- sample strings -> reference INT4-QUANTIZED
                            embeddings (same per-row symmetric int4
                            scheme as internal/embed/gen/generate.py,
                            applied here in Python, then encoded via
                            tokenize -> gather -> mean-pool -> L2-normalize,
                            mirroring model2vec.StaticModel.encode)

The embedding reference intentionally quantizes the matrix to int4
before encoding: the golden gate compares the Go implementation's int4
Embed() against this int4 Python reference (>= 0.999 cosine), which
validates that the Go tokenize/gather/pool/normalize pipeline and int4
dequantization match this reference bit-for-bit-ish (only float
accumulation order differs) -- NOT that int4 has no quality loss vs
the original fp32 model (that tradeoff is a separate, spike-validated
decision; see internal/embed/README.md).

Requires network access (or a warm huggingface_hub cache) and the
model2vec/tokenizers packages; run at fixture-generation time only.
The fixtures are committed and loaded by embed_test.go with no network
or model download at test time.

Usage:
    pip install model2vec tokenizers huggingface_hub numpy
    python3 internal/embed/gen/generate_golden.py
"""

from __future__ import annotations

import json

import numpy as np
from model2vec import StaticModel

MODEL_ID = "minishlab/potion-retrieval-32M"

TOKENIZER_SAMPLES = [
    "café résumé naïve",
    "Hello, World!  Multi   space",
    "don't stop believin'",
    "unbelievable subword-splitting",
    "COVID-19",
    "",
    "汉字测试chinese chars",
    "special_chars: @#$%^&*()",
    "numbers 12345 and dates 2026-07-10",
    "México Köln naïve",
    "price is €50",
    "a 3×3 grid",
    "see the arrow → here",
    "© 2026 acme",
]

EMBEDDING_SAMPLES = [
    "auth approach",
    "API bearer token authentication",
    "completely unrelated topic about gardening",
    "how do we store data",
    "semantic search implementation",
    "",
    "The quick brown fox jumps over the lazy dog!",
    "COVID-19 pandemic response strategies",
    "asdkjfhaslkdjfh unknownwordxyz123",
    "wifi reconnect stability issues",
    "coverage requirements for tests",
    "model selection for embeddings",
    "branch and PR workflow",
    "memory allocation on constrained devices",
    "why pure Go",
    "price is €50",
    "a 3×3 grid",
    "see the arrow → here",
    "© 2026 acme",
]

OUT_TOKENIZER_JSON = "internal/embed/testdata/tokenizer_parity.json"
OUT_EMBEDDING_JSON = "internal/embed/testdata/embedding_golden.json"


def quantize_int4_per_row(mat: np.ndarray) -> np.ndarray:
    """Per-row symmetric int4 (values clipped to [-7,7]), dequantized.

    Mirrors internal/embed/gen/generate.py's quantization exactly
    (same scale formula, same rounding/clipping), returning the
    dequantized fp32 matrix that internal/embed/asset.go's
    unpack-and-dequant reproduces at runtime.
    """
    absmax = np.max(np.abs(mat), axis=1)
    absmax[absmax == 0] = 1.0
    scales = (absmax / 7.0).astype(np.float32)
    quantized = np.clip(np.round(mat / scales[:, None]), -7, 7).astype(np.float32)
    return quantized * scales[:, None]


def encode_with_matrix(model: StaticModel, matrix: np.ndarray, texts: list[str], max_length: int = 512) -> np.ndarray:
    """Tokenize -> gather -> mean-pool -> L2-normalize with a substitute
    embedding matrix, mirroring model2vec.StaticModel.encode exactly
    (and internal/embed's Embed())."""
    ids_batches = model.tokenize(sentences=texts, max_length=max_length)
    out = []
    for id_list in ids_batches:
        if id_list:
            emb = matrix[id_list]
            out.append(emb.mean(axis=0))
        else:
            out.append(np.zeros(matrix.shape[1], dtype=np.float32))
    arr = np.stack(out)
    norm = np.linalg.norm(arr, axis=1, keepdims=True) + 1e-32
    return (arr / norm).astype(np.float32)


def main() -> None:
    model = StaticModel.from_pretrained(MODEL_ID)

    tok_fixtures = []
    for text in TOKENIZER_SAMPLES:
        ids = model.tokenize([text])[0]
        tok_fixtures.append({"text": text, "ids": [int(i) for i in ids]})
    with open(OUT_TOKENIZER_JSON, "w", encoding="utf-8") as out:
        json.dump(tok_fixtures, out, indent=2, ensure_ascii=False)
    print(f"wrote {OUT_TOKENIZER_JSON} ({len(tok_fixtures)} samples)")

    fp32_matrix = model.embedding.astype(np.float32)
    int4_matrix = quantize_int4_per_row(fp32_matrix)

    embs = encode_with_matrix(model, int4_matrix, EMBEDDING_SAMPLES)
    emb_fixtures = []
    for text, vec in zip(EMBEDDING_SAMPLES, embs):
        emb_fixtures.append({"text": text, "embedding": [float(x) for x in vec]})
    with open(OUT_EMBEDDING_JSON, "w", encoding="utf-8") as out:
        json.dump(emb_fixtures, out, ensure_ascii=False)
    print(f"wrote {OUT_EMBEDDING_JSON} ({len(emb_fixtures)} samples)")


if __name__ == "__main__":
    main()
