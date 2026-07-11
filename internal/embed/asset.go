package embed

import (
	"bufio"
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

//go:embed data/model.bin
var modelBin []byte

//go:embed data/vocab.txt
var vocabTxt string

const (
	assetMagic            = "M2V1"
	assetHeaderSize       = 20 // magic(4) + version(4) + vocab(4) + dim(4) + quantScheme(4)
	quantSchemeInt8PerRow = 0  // prior scheme; no longer emitted, kept for docs/back-compat
	quantSchemeInt4PerRow = 2
	unkToken              = "[UNK]"
)

// asset is the decoded int4 + per-row-scale embedding matrix, in the
// format written by internal/embed/gen/generate.py:
//
//	magic       [4]byte  "M2V1"
//	version     uint32
//	vocab       uint32
//	dim         uint32
//	quantScheme uint32   2 = int4 per-row symmetric (packed 2/byte)
//	data        [vocab*dim/2]byte  row-major int4-packed matrix
//	scales      [vocab]float32     per-row dequantization scale
//
// Each byte packs two 4-bit signed (two's complement, range [-7,7])
// values: the low nibble is the even column, the high nibble is the
// odd column (matches internal/embed/gen/generate.py's pack_int4).
type asset struct {
	vocab int
	dim   int
	data  []byte    // row-major, vocab*dim/2, int4-packed (2 values/byte)
	scale []float32 // per-row dequantization scale
}

func loadAsset(raw []byte) (*asset, error) {
	if len(raw) < assetHeaderSize {
		return nil, fmt.Errorf("embed: asset too small: %d bytes", len(raw))
	}
	if string(raw[:4]) != assetMagic {
		return nil, fmt.Errorf("embed: bad asset magic %q", raw[:4])
	}
	version := binary.LittleEndian.Uint32(raw[4:8])
	if version != 1 {
		return nil, fmt.Errorf("embed: unsupported asset version %d", version)
	}
	vocab := int(binary.LittleEndian.Uint32(raw[8:12]))
	dim := int(binary.LittleEndian.Uint32(raw[12:16]))
	quantScheme := binary.LittleEndian.Uint32(raw[16:20])
	if quantScheme != quantSchemeInt4PerRow {
		return nil, fmt.Errorf("embed: unsupported quant scheme %d", quantScheme)
	}
	if dim%2 != 0 {
		return nil, fmt.Errorf("embed: int4 asset requires an even dim, got %d", dim)
	}

	dataLen := vocab * dim / 2
	scaleOff := assetHeaderSize + dataLen
	scaleLen := vocab * 4
	wantLen := scaleOff + scaleLen
	if len(raw) != wantLen {
		return nil, fmt.Errorf("embed: asset size mismatch: got %d bytes, want %d", len(raw), wantLen)
	}

	data := make([]byte, dataLen)
	copy(data, raw[assetHeaderSize:scaleOff])

	scale := make([]float32, vocab)
	for i := range scale {
		off := scaleOff + i*4
		bits := binary.LittleEndian.Uint32(raw[off : off+4])
		scale[i] = math.Float32frombits(bits)
	}

	return &asset{vocab: vocab, dim: dim, data: data, scale: scale}, nil
}

// unpackInt4 extracts the signed int4 value (range [-8,7], though
// generate.py only ever writes [-7,7]) for column j of a packed row,
// sign-extending the nibble: low nibble = even j, high nibble = odd j.
func unpackInt4(rowData []byte, j int) int8 {
	b := rowData[j/2]
	var nib byte
	if j%2 == 0 {
		nib = b & 0x0F
	} else {
		nib = (b >> 4) & 0x0F
	}
	v := int8(nib)
	if v >= 8 {
		v -= 16
	}
	return v
}

// row returns the dequantized embedding row for token id.
func (a *asset) row(id int32) []float32 {
	out := make([]float32, a.dim)
	s := a.scale[id]
	base := int(id) * a.dim / 2
	rowData := a.data[base : base+a.dim/2]
	for i := 0; i < a.dim; i++ {
		out[i] = float32(unpackInt4(rowData, i)) * s
	}
	return out
}

// accumulateRow dequantizes token id's int4-packed embedding row and
// adds it element-wise into dst (len(dst) must equal a.dim), without
// allocating a per-token row slice.
func (a *asset) accumulateRow(id int32, dst []float32) {
	s := a.scale[id]
	base := int(id) * a.dim / 2
	rowData := a.data[base : base+a.dim/2]
	for i := 0; i < a.dim; i++ {
		dst[i] += float32(unpackInt4(rowData, i)) * s
	}
}

// loadVocab parses a WordPiece vocab.txt (one token per line, ordered
// by token id) into a token->id map and returns the [UNK] token id.
func loadVocab(txt string) (map[string]int32, int32, error) {
	vocab := make(map[string]int32)
	scanner := bufio.NewScanner(strings.NewReader(txt))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var id int32
	for scanner.Scan() {
		vocab[scanner.Text()] = id
		id++
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("embed: reading vocab: %w", err)
	}
	unkID, ok := vocab[unkToken]
	if !ok {
		return nil, 0, fmt.Errorf("embed: vocab missing %s token", unkToken)
	}
	return vocab, unkID, nil
}
