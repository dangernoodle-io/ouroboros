package embed

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeLowercasesAndStripsAccents(t *testing.T) {
	require.Equal(t, "cafe resume naive", normalize("café résumé naïve"))
}

func TestNormalizeCollapsesWhitespace(t *testing.T) {
	require.Equal(t, "hello world", normalize("Hello\tWorld"))
}

func TestNormalizeDropsControlChars(t *testing.T) {
	withNUL := string([]rune{'H', 'e', 'l', 'l', 'o', rune(0), 'W', 'o', 'r', 'l', 'd'})
	require.Equal(t, "helloworld", normalize(withNUL))
}

func TestNormalizeWrapsChineseChars(t *testing.T) {
	got := normalize("a汉b")
	require.Equal(t, "a 汉 b", got)
}

func TestPreTokenizeSplitsOnWhitespaceAndPunctuation(t *testing.T) {
	require.Equal(t, []string{"hello", ",", "world", "!"}, preTokenize("hello, world!"))
	require.Equal(t, []string{"don", "'", "t"}, preTokenize("don't"))
	require.Nil(t, preTokenize(""))
	require.Nil(t, preTokenize("   "))
}

func TestIsChineseChar(t *testing.T) {
	require.True(t, isChineseChar('汉'))
	require.False(t, isChineseChar('a'))
}

func TestIsBertPunctuation(t *testing.T) {
	for _, r := range []rune{'!', '$', '+', '<', '=', '>', '^', '`', '|', '~', ',', '.'} {
		require.Truef(t, isBertPunctuation(r), "expected %q to be punctuation", r)
	}
	require.False(t, isBertPunctuation('a'))
	require.False(t, isBertPunctuation('9'))
}

func TestWordpieceTokenizeUnknownWord(t *testing.T) {
	vocab := map[string]int32{"[UNK]": 0, "hel": 1, "##lo": 2}
	w := newWordpiece(vocab, 0)
	// "hello" splits as hel + ##lo.
	require.Equal(t, []int32{1, 2}, w.tokenizeWord("hello"))
	// "xyz" has no valid split -> whole word is UNK.
	require.Equal(t, []int32{0}, w.tokenizeWord("xyz"))
	// Empty word -> no tokens.
	require.Nil(t, w.tokenizeWord(""))
}

func TestWordpieceTokenizeDropsUnknownTokensFromSentence(t *testing.T) {
	vocab := map[string]int32{"[UNK]": 0, "hel": 1, "##lo": 2, "world": 3}
	w := newWordpiece(vocab, 0)
	got := w.tokenize("hello xyz world")
	require.Equal(t, []int32{1, 2, 3}, got)
}

func TestWordpieceTokenizeOverlongWord(t *testing.T) {
	vocab := map[string]int32{"[UNK]": 0}
	w := newWordpiece(vocab, 0)
	long := make([]rune, wordpieceMaxInputChars+1)
	for i := range long {
		long[i] = 'a'
	}
	require.Equal(t, []int32{0}, w.tokenizeWord(string(long)))
}
