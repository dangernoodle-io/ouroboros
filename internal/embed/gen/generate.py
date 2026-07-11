#!/usr/bin/env python3
"""Generate the int4-quantized static-embedding asset for internal/embed.

Reads the Model2Vec `minishlab/potion-retrieval-32M` embedding matrix
(a [vocab, 512] float32 sentence-transformers StaticEmbedding) and its
WordPiece vocabulary, quantizes the matrix to int4 with a per-row
(per-token) symmetric scale (2 values packed per byte), and writes:

  internal/embed/data/model.bin  -- header + int4-packed matrix + per-row scales
  internal/embed/data/vocab.txt  -- one token per line, ordered by token id

Requires network access (or a warm huggingface_hub cache) to fetch the
model; this script runs at asset-generation time only, never at test or
build time -- the generated files are committed and //go:embed'd.

Usage:
    pip install huggingface_hub safetensors numpy
    python3 internal/embed/gen/generate.py

Asset format (model.bin), all integers little-endian:
    magic       [4]byte  "M2V1"
    version     uint32   1
    vocab       uint32   number of rows (tokens)
    dim         uint32   embedding dimension (512)
    quantScheme uint32   2 = int4 per-row symmetric (packed 2/byte)
    data        [vocab*dim/2]uint8  row-major int4-packed matrix
    scales      [vocab]float32      per-row dequantization scale

Dequantization: unpack the 4-bit nibble for column j of row i (low
nibble = even j, high nibble = odd j), sign-extend to [-7,7], then
value = nibble * scales[i].

Note: QUANT_SCHEME_INT8_PER_ROW (0) is kept defined for back-compat
documentation of the prior asset format; this generator only emits the
int4 (QUANT_SCHEME_INT4_PER_ROW = 2) scheme now.
"""

from __future__ import annotations

import struct

import numpy as np
import safetensors
from huggingface_hub import snapshot_download

MODEL_ID = "minishlab/potion-retrieval-32M"
MAGIC = b"M2V1"
VERSION = 1
QUANT_SCHEME_INT8_PER_ROW = 0  # prior scheme, no longer emitted; kept for docs
QUANT_SCHEME_INT4_PER_ROW = 2

OUT_MODEL_BIN = "internal/embed/data/model.bin"
OUT_VOCAB_TXT = "internal/embed/data/vocab.txt"


def pack_int4(q: np.ndarray) -> np.ndarray:
    """Pack an (rows, cols) int8 array with values in [-7,7] into 2/byte.

    Column j -> low nibble if j even, high nibble if j odd (matches
    internal/embed/asset.go's unpack order). cols must be even.
    """
    rows, cols = q.shape
    if cols % 2 != 0:
        raise ValueError(f"int4 packing requires an even column count, got {cols}")
    qu = (q.astype(np.int16) & 0xF).astype(np.uint8)  # 4-bit two's complement repr
    lo = qu[:, 0::2]
    hi = qu[:, 1::2]
    return (lo | (hi << 4)).astype(np.uint8)


def main() -> None:
    folder = snapshot_download(MODEL_ID, repo_type="model")

    with safetensors.safe_open(f"{folder}/model.safetensors", framework="numpy") as f:
        embeddings = f.get_tensor("embeddings").astype(np.float32)

    vocab_size, dim = embeddings.shape
    print(f"loaded embeddings: vocab={vocab_size} dim={dim}")

    with open(f"{folder}/vocab.txt", encoding="utf-8") as vf:
        vocab_lines = vf.read()
    n_vocab_lines = len(vocab_lines.splitlines())
    if n_vocab_lines != vocab_size:
        raise ValueError(f"vocab.txt has {n_vocab_lines} lines but embeddings has {vocab_size} rows")

    # Per-row (per-token) symmetric int4 quantization: values clipped to
    # [-7,7] (not [-8,7]) to keep the range perfectly symmetric.
    absmax = np.max(np.abs(embeddings), axis=1)
    absmax[absmax == 0] = 1.0  # avoid div-by-zero for an all-zero row
    scales = (absmax / 7.0).astype(np.float32)
    quantized = np.round(embeddings / scales[:, None])
    quantized = np.clip(quantized, -7, 7).astype(np.int8)
    packed = pack_int4(quantized)

    # Report achieved per-row reconstruction fidelity.
    dequantized = quantized.astype(np.float32) * scales[:, None]
    num = np.sum(embeddings * dequantized, axis=1)
    den = np.linalg.norm(embeddings, axis=1) * np.linalg.norm(dequantized, axis=1)
    den[den == 0] = 1.0
    row_cos = num / den
    print(f"per-row cosine reconstruction: min={row_cos.min():.6f} mean={row_cos.mean():.6f}")

    with open(OUT_MODEL_BIN, "wb") as out:
        out.write(MAGIC)
        out.write(struct.pack("<I", VERSION))
        out.write(struct.pack("<I", vocab_size))
        out.write(struct.pack("<I", dim))
        out.write(struct.pack("<I", QUANT_SCHEME_INT4_PER_ROW))
        out.write(packed.tobytes(order="C"))
        out.write(scales.tobytes(order="C"))

    with open(OUT_VOCAB_TXT, "w", encoding="utf-8") as out:
        out.write(vocab_lines)

    asset_bytes = 20 + packed.nbytes + scales.nbytes
    print(f"wrote {OUT_MODEL_BIN} ({asset_bytes} bytes)")
    print(f"wrote {OUT_VOCAB_TXT}")


if __name__ == "__main__":
    main()
