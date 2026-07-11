package embed

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func validHeader(vocab, dim, quantScheme uint32) []byte {
	b := make([]byte, assetHeaderSize)
	copy(b[:4], assetMagic)
	binary.LittleEndian.PutUint32(b[4:8], 1)
	binary.LittleEndian.PutUint32(b[8:12], vocab)
	binary.LittleEndian.PutUint32(b[12:16], dim)
	binary.LittleEndian.PutUint32(b[16:20], quantScheme)
	return b
}

func TestLoadAssetTooSmall(t *testing.T) {
	_, err := loadAsset([]byte{1, 2, 3})
	require.Error(t, err)
}

func TestLoadAssetBadMagic(t *testing.T) {
	h := validHeader(1, 2, quantSchemeInt4PerRow)
	h[0] = 'X'
	raw := slices.Concat(h, make([]byte, 1+4))
	_, err := loadAsset(raw)
	require.ErrorContains(t, err, "bad asset magic")
}

func TestLoadAssetBadVersion(t *testing.T) {
	h := make([]byte, assetHeaderSize)
	copy(h[:4], assetMagic)
	binary.LittleEndian.PutUint32(h[4:8], 2)
	raw := slices.Concat(h, make([]byte, 1+4))
	_, err := loadAsset(raw)
	require.ErrorContains(t, err, "unsupported asset version")
}

func TestLoadAssetBadQuantScheme(t *testing.T) {
	h := validHeader(1, 2, 7)
	raw := slices.Concat(h, make([]byte, 1+4))
	_, err := loadAsset(raw)
	require.ErrorContains(t, err, "unsupported quant scheme")
}

func TestLoadAssetRejectsInt8Scheme(t *testing.T) {
	// The prior int8 scheme (0) is no longer accepted by loadAsset.
	h := validHeader(1, 2, quantSchemeInt8PerRow)
	raw := slices.Concat(h, make([]byte, 1+4))
	_, err := loadAsset(raw)
	require.ErrorContains(t, err, "unsupported quant scheme")
}

func TestLoadAssetOddDim(t *testing.T) {
	h := validHeader(1, 3, quantSchemeInt4PerRow)
	raw := slices.Concat(h, make([]byte, 2+4)) // doesn't matter, rejected before size check
	_, err := loadAsset(raw)
	require.ErrorContains(t, err, "even dim")
}

func TestLoadAssetSizeMismatch(t *testing.T) {
	h := validHeader(1, 2, quantSchemeInt4PerRow)
	raw := slices.Concat(h, make([]byte, 1)) // missing the scale bytes
	_, err := loadAsset(raw)
	require.ErrorContains(t, err, "asset size mismatch")
}

func TestLoadAssetRoundTrip(t *testing.T) {
	h := validHeader(2, 4, quantSchemeInt4PerRow)
	// 2 rows x 4 dim int4, packed 2 vals/byte -> 2 bytes/row.
	// row0: [1, 2, -3, 4]  -> nibbles (1,2)=0x21, (-3,4)=(0xD,0x4)=0x4D
	// row1: [-7, 7, 0, -1] -> nibbles (-7,7)=(0x9,0x7)=0x79, (0,-1)=(0x0,0xF)=0xF0
	data := []byte{0x21, 0x4D, 0x79, 0xF0}
	scales := make([]byte, 8)
	binary.LittleEndian.PutUint32(scales[0:4], math.Float32bits(2.0))
	binary.LittleEndian.PutUint32(scales[4:8], math.Float32bits(0.5))
	raw := slices.Concat(h, data, scales)

	a, err := loadAsset(raw)
	require.NoError(t, err)
	require.Equal(t, 2, a.vocab)
	require.Equal(t, 4, a.dim)

	row0 := a.row(0)
	require.Equal(t, []float32{2, 4, -6, 8}, row0)

	row1 := a.row(1)
	require.Equal(t, []float32{-3.5, 3.5, 0, -0.5}, row1)
}

func TestAccumulateRow(t *testing.T) {
	h := validHeader(1, 4, quantSchemeInt4PerRow)
	data := []byte{0x21, 0x4D} // row0: [1, 2, -3, 4]
	scales := make([]byte, 4)
	binary.LittleEndian.PutUint32(scales[0:4], math.Float32bits(2.0))
	raw := slices.Concat(h, data, scales)

	a, err := loadAsset(raw)
	require.NoError(t, err)

	dst := []float32{10, 10, 10, 10}
	a.accumulateRow(0, dst)
	require.Equal(t, []float32{12, 14, 4, 18}, dst)
}

func TestUnpackInt4SignExtend(t *testing.T) {
	// byte 0xF0: low nibble 0x0 -> 0, high nibble 0xF -> -1
	row := []byte{0xF0}
	require.Equal(t, int8(0), unpackInt4(row, 0))
	require.Equal(t, int8(-1), unpackInt4(row, 1))
}

func TestLoadVocabMissingUnk(t *testing.T) {
	_, _, err := loadVocab("[PAD]\nhello\n")
	require.ErrorContains(t, err, "missing")
}

func TestLoadVocabOrdering(t *testing.T) {
	vocab, unkID, err := loadVocab("[PAD]\n[UNK]\nhello\n##world\n")
	require.NoError(t, err)
	require.Equal(t, int32(1), unkID)
	require.Equal(t, int32(0), vocab["[PAD]"])
	require.Equal(t, int32(2), vocab["hello"])
	require.Equal(t, int32(3), vocab["##world"])
}
