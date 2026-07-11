package embed

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// wordpiece is a pure-Go re-implementation of the standard BERT
// normalizer + BertPreTokenizer + WordPiece pipeline used by the
// potion-retrieval-32M tokenizer (base model: baai/bge-base-en-v1.5).
//
// Pipeline (mirrors huggingface `tokenizers`' BertNormalizer +
// BertPreTokenizer + WordPiece model exactly, for the subset of
// behavior this model relies on):
//  1. clean text: drop control chars, collapse whitespace-class runes to ' '
//  2. wrap CJK codepoints in spaces so they tokenize as isolated chars
//  3. NFD-normalize and strip combining marks (accent stripping)
//  4. lowercase
//  5. split on whitespace, then split runs of punctuation into single-rune tokens
//  6. WordPiece: greedy longest-match per word, "##" continuation prefix,
//     whole word -> [UNK] if no split point is found
type wordpiece struct {
	vocab      map[string]int32
	unkID      int32
	maxWordLen int
}

const wordpieceMaxInputChars = 100

func newWordpiece(vocab map[string]int32, unkID int32) *wordpiece {
	return &wordpiece{vocab: vocab, unkID: unkID, maxWordLen: wordpieceMaxInputChars}
}

// tokenize returns the WordPiece token ids for text, mirroring
// model2vec.StaticModel.tokenize: no special tokens ([CLS]/[SEP]) are
// added, and [UNK] tokens are dropped entirely rather than embedded.
func (w *wordpiece) tokenize(text string) []int32 {
	normalized := normalize(text)
	words := preTokenize(normalized)

	ids := make([]int32, 0, len(words))
	for _, word := range words {
		for _, id := range w.tokenizeWord(word) {
			if id == w.unkID {
				continue
			}
			ids = append(ids, id)
		}
	}
	return ids
}

// tokenizeWord runs greedy longest-match WordPiece on a single
// whitespace/punctuation-delimited word.
func (w *wordpiece) tokenizeWord(word string) []int32 {
	runes := []rune(word)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) > w.maxWordLen {
		return []int32{w.unkID}
	}

	var out []int32
	start := 0
	for start < len(runes) {
		end := len(runes)
		var matchID int32 = -1
		for start < end {
			substr := string(runes[start:end])
			if start > 0 {
				substr = "##" + substr
			}
			if id, ok := w.vocab[substr]; ok {
				matchID = id
				break
			}
			end--
		}
		if matchID == -1 {
			return []int32{w.unkID}
		}
		out = append(out, matchID)
		start = end
	}
	return out
}

// normalize mirrors BertNormalizer{clean_text, handle_chinese_chars,
// strip_accents (via lowercase), lowercase}.
func normalize(text string) string {
	var b strings.Builder
	b.Grow(len(text))

	for _, r := range text {
		switch {
		case r == 0 || r == 0xFFFD || isControl(r):
			continue
		case isBertWhitespace(r):
			b.WriteRune(' ')
		case isChineseChar(r):
			b.WriteRune(' ')
			b.WriteRune(r)
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}

	return strings.ToLower(stripAccents(b.String()))
}

// stripAccents NFD-decomposes text and drops combining marks (Unicode
// category Mn), mirroring BERT's _run_strip_accents.
func stripAccents(s string) string {
	decomposed := norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isControl mirrors BERT's _is_control: \t, \n, \r are never control;
// everything in Unicode categories Cc/Cf/Co/Cs is. Python's reference
// (`unicodedata.category(char).startswith("C")`) also matches Cn
// (unassigned codepoints); this intentionally does not, since Go's
// unicode package has no Cn table, and real KB/backlog text won't
// contain unassigned codepoints in practice.
func isControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs)
}

// isBertWhitespace mirrors BERT's _is_whitespace.
func isBertWhitespace(r rune) bool {
	if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// isBertPunctuation mirrors BERT's _is_punctuation: the ASCII ranges
// that aren't in Unicode's P* categories (e.g. '$', '+', '<', '=', '>',
// '^', '`', '|', '~') are explicitly included, matching BERT's original
// (non-Unicode-category) definition, plus Unicode category P*. Unlike
// a naive port, this deliberately does NOT also match Unicode S*
// (Sm/Sc/Sk/So: e.g. €, ©, ±, ×, °, →) — the reference only checks
// P*, so non-ASCII symbols are ordinary word characters here.
func isBertPunctuation(r rune) bool {
	if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) || (r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
		return true
	}
	return unicode.IsPunct(r)
}

// isChineseChar mirrors BERT's _is_chinese_char codepoint ranges.
func isChineseChar(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x20000 && r <= 0x2A6DF) ||
		(r >= 0x2A700 && r <= 0x2B73F) ||
		(r >= 0x2B740 && r <= 0x2B81F) ||
		(r >= 0x2B820 && r <= 0x2CEAF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0x2F800 && r <= 0x2FA1F)
}

// preTokenize mirrors BertPreTokenizer: split on whitespace, then split
// runs of punctuation into single-rune tokens.
func preTokenize(text string) []string {
	var words []string
	var cur strings.Builder

	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}

	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			flush()
		case isBertPunctuation(r):
			flush()
			words = append(words, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()

	return words
}
