package embed

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRetrievalRankingConceptQuery directly validates the OU-68
// motivating bug at the Go embedder level: a query that shares NO
// content-word vocabulary with the semantically relevant document
// must still rank that document above clearly unrelated distractors.
// This is exactly the case keyword/FTS search misses (no shared
// tokens to match on) but a semantic embedding should catch via
// meaning alone. The product end-to-end path (the search MCP tool
// over the wire) is covered separately by OU-282's acceptance-harness
// test; this test isolates the claim to the embedder itself.
func TestRetrievalRankingConceptQuery(t *testing.T) {
	type scenario struct {
		name        string
		query       string
		relevant    string
		distractors []string
	}

	scenarios := []scenario{
		{
			name:     "auth approach",
			query:    "auth approach",
			relevant: "the service verifies a JSON Web Token bearer credential on every request",
			distractors: []string{
				"remember to water the greenhouse tomatoes twice a day",
				"wifi reconnect backoff needs a longer delay between retries",
				"heap allocation on microcontrollers should be avoided at runtime",
				"the quarterly invoice from the vendor was paid last week",
			},
		},
		{
			name:     "how is data stored",
			query:    "how is data stored",
			relevant: "records persist to a SQLite database file on local disk",
			distractors: []string{
				"the hiking trail closes early during wildfire season",
				"replace the smoke detector battery every six months",
				"the marathon route passes through five different neighborhoods",
				"paint the fence before the rainy season begins",
			},
		},
		{
			name:     "run the tests with coverage",
			query:    "run the tests with coverage",
			relevant: "execute the unit suite and report line-coverage percentages",
			distractors: []string{
				"the bakery sells fresh sourdough loaves every morning",
				"the museum exhibit features ancient pottery from the region",
				"turn off the porch light before leaving for vacation",
				"the orchestra rehearsed the symphony for three hours",
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			queryVec := Embed(sc.query)
			relevantScore := Cosine(queryVec, Embed(sc.relevant))

			bestDistractorScore := float32(-1)
			var bestDistractor string
			for _, d := range sc.distractors {
				score := Cosine(queryVec, Embed(d))
				t.Logf("distractor %q: cosine=%.4f", d, score)
				if score > bestDistractorScore {
					bestDistractorScore = score
					bestDistractor = d
				}
			}
			t.Logf("query=%q relevant=%q cosine=%.4f (best distractor %q cosine=%.4f, margin=%.4f)",
				sc.query, sc.relevant, relevantScore, bestDistractor, bestDistractorScore,
				relevantScore-bestDistractorScore)

			require.Greaterf(t, relevantScore, bestDistractorScore,
				"relevant doc %q (cosine=%.4f) must outrank best distractor %q (cosine=%.4f) for query %q",
				sc.relevant, relevantScore, bestDistractor, bestDistractorScore, sc.query)
		})
	}
}
