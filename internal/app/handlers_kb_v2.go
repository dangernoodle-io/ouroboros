package app

import (
	"context"
	"fmt"
	"strconv"

	"github.com/dangernoodle-io/mcpkit/mcpx"

	"dangernoodle.io/ouroboros/internal/kb"
)

// errKBEntriesRequired is the verbatim message the old kb write handler's
// empty-entries check returned for both an omitted entries key and
// an empty entries[] array (see kbInput.Entries's comment).
const errKBEntriesRequired = "entries array is required (batch-only mode)"

// handleKBV2 splits entries[] on id presence (create/natural-key-upsert vs
// id-addressed partial update) and delegates the whole batch to
// kb.WriteAndUpdateBatch for the atomic single-tx write + [[Title]] autolink
// + FTS rebuild (see internal/kb/batch.go -- not reimplemented here). st.db
// is read at call time (not at construction time) — see serverState's doc
// comment.
func handleKBV2(st *serverState) mcpx.Handler[kbInput, any] {
	return func(_ context.Context, _ *mcpx.CallToolRequest, in kbInput) (*mcpx.CallToolResult, any, error) {
		if len(in.Entries) == 0 {
			return mcpx.ErrorResult(errKBEntriesRequired), nil, nil
		}

		creates := make([]kb.Entry, 0, len(in.Entries))
		var updates []kb.EntryUpdate
		for _, e := range in.Entries {
			id, present, err := parseKBIDV2(e.ID)
			if err != nil {
				// Abort the whole call before any DB write -- the
				// create/update slices built so far are never passed to
				// kb.WriteAndUpdateBatch.
				return mcpx.ErrorResult(err.Error()), nil, nil
			}
			if !present {
				creates = append(creates, kbEntryCreateV2(e))
				continue
			}
			updates = append(updates, kbEntryUpdateV2(id, e))
		}

		createResults, updateResults, err := kb.WriteAndUpdateBatch(st.db, creates, updates, "")
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}

		results := make([]kb.PutResult, 0, len(createResults)+len(updateResults))
		results = append(results, createResults...)
		results = append(results, updateResults...)

		return jsonResultV2(results)
	}
}

// parseKBIDV2 is parseKBEntryID's typed-input counterpart, operating on the
// raw decoded `any` value of kbEntryInput.ID (see its comment for why ID is
// `any` and not a custom-decode type). Reproduces parseKBEntryID verbatim:
// a JSON number (exact positive int64) or a numeric string routes to update;
// anything else present (zero, negative, non-integer, non-scalar) is a hard
// error; an absent (nil) id selects create.
func parseKBIDV2(raw any) (id int64, present bool, err error) {
	if raw == nil {
		return 0, false, nil
	}

	switch v := raw.(type) {
	case float64:
		if v != float64(int64(v)) || v <= 0 {
			return 0, true, fmt.Errorf("invalid id: %v", raw)
		}
		return int64(v), true, nil
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return 0, true, fmt.Errorf("invalid id: %v", raw)
		}
		return n, true, nil
	default:
		return 0, true, fmt.Errorf("invalid id: %v", raw)
	}
}

// kbEntryCreateV2 builds a kb.Entry for the id-absent create/upsert path,
// mirroring parseKBEntryCreate: an unset (nil) field maps to "" / nil, the
// zero value for an absent field.
func kbEntryCreateV2(e kbEntryInput) kb.Entry {
	return kb.Entry{
		Type:     strOrEmptyV2(e.Type),
		Project:  strOrEmptyV2(e.Project),
		Category: strOrEmptyV2(e.Category),
		Title:    strOrEmptyV2(e.Title),
		Content:  strOrEmptyV2(e.Content),
		Notes:    strOrEmptyV2(e.Notes),
		Tags:     tagsOrNilV2(e.Tags),
		Metadata: metaOrNilV2(e.Metadata),
	}
}

// kbEntryUpdateV2 builds a kb.EntryUpdate for the id-addressed update path.
// kbEntryInput's pointer fields already match kb.EntryUpdate's shape
// (pointer = presence), so this is a direct field-for-field pass-through --
// no deref needed, unlike the create path.
func kbEntryUpdateV2(id int64, e kbEntryInput) kb.EntryUpdate {
	return kb.EntryUpdate{
		ID:       id,
		Type:     e.Type,
		Project:  e.Project,
		Category: e.Category,
		Title:    e.Title,
		Content:  e.Content,
		Notes:    e.Notes,
		Tags:     e.Tags,
		Metadata: e.Metadata,
	}
}

func strOrEmptyV2(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func tagsOrNilV2(p *[]string) []string {
	if p == nil {
		return nil
	}
	return *p
}

func metaOrNilV2(p *map[string]string) map[string]string {
	if p == nil {
		return nil
	}
	return *p
}
