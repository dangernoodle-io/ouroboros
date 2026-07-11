package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dangernoodle-io/mcpkit/mcpx"

	"dangernoodle.io/ouroboros/internal/roadmap"
)

// errRoadmapProjectRequired/errRoadmapOpRequired/errRoadmapIDRequired/
// errRoadmapPositionRequired are the verbatim messages the old roadmap write
// handler's per-op validation returned for a missing project/op/id/position
// -- reproduced here since roadmapInput schema-declares every one of these
// fields omitempty (see roadmapInput's comment).
const (
	errRoadmapProjectRequired  = "project is required"
	errRoadmapOpRequired       = `op is required: must be "add", "update", "move", "reorder", "done", or "remove"`
	errRoadmapIDRequired       = "id is required"
	errRoadmapPositionRequired = "position is required"
)

// handleRoadmapV2 (OU-4) dispatches op=add|update|move|reorder|done|remove
// against roadmapInput's single flat arg set, reproducing the old roadmap
// write handler's per-op validation and verbatim errors exactly (project
// checked first, matching the old handler's dispatch order), then delegates
// every mutation to roadmap.Mutate (one tx per op:
// load->mutate->save->commit, rollback on error, FTS rebuild post-commit)
// -- reused, not reimplemented. st.db is read at call time (not at
// construction time) — see serverState's doc comment.
func handleRoadmapV2(st *serverState) mcpx.Handler[roadmapInput, any] {
	return func(_ context.Context, _ *mcpx.CallToolRequest, in roadmapInput) (*mcpx.CallToolResult, any, error) {
		if in.Project == "" {
			return mcpx.ErrorResult(errRoadmapProjectRequired), nil, nil
		}

		switch in.Op {
		case "add":
			return handleRoadmapAddV2(st.db, in)
		case "update":
			return handleRoadmapUpdateV2(st.db, in)
		case "move":
			return handleRoadmapMoveV2(st.db, in)
		case "reorder":
			return handleRoadmapReorderV2(st.db, in)
		case "done":
			return handleRoadmapDoneV2(st.db, in)
		case "remove":
			return handleRoadmapRemoveV2(st.db, in)
		default:
			return mcpx.ErrorResult(errRoadmapOpRequired), nil, nil
		}
	}
}

// intSliceOrNilV2 returns *p if non-nil, else nil -- roadmapInput.KB's
// *[]int deref-or-default counterpart to tagsOrNilV2 (handlers_kb_v2.go),
// which already serves the *[]string shape roadmapInput.Ticket shares.
func intSliceOrNilV2(p *[]int) []int {
	if p == nil {
		return nil
	}
	return *p
}

// blockersFromInputV2 builds []roadmap.Blocker from a []blockerInput slice,
// reproducing parseBlockers' per-element hard-error verbatim messages
// (missing project/ref). A non-object blocked_by[] element can't reach here
// at all -- go-sdk's decode rejects the whole call with a decode error
// instead (ACCEPTED DIVERGENCE, see blockerInput's comment).
func blockersFromInputV2(in []blockerInput) ([]roadmap.Blocker, error) {
	if len(in) == 0 {
		return nil, nil
	}

	result := make([]roadmap.Blocker, 0, len(in))
	for i, b := range in {
		if b.Project == "" {
			return nil, fmt.Errorf("blocked_by[%d] missing project", i)
		}
		if b.Ref == "" {
			return nil, fmt.Errorf("blocked_by[%d] missing ref", i)
		}
		result = append(result, roadmap.Blocker{Project: b.Project, Ref: b.Ref, Note: b.Note})
	}
	return result, nil
}

// handleRoadmapAddV2 mirrors handleRoadmapAdd: validates section, parses
// blockers, and builds a roadmap.Item from roadmapInput's pointer fields
// read as "value or absent -> zero" (deref-or-default), then adds it via
// roadmap.AddItem inside roadmap.Mutate. position is presence-signaled,
// mirroring move/reorder -- item.Position is never an ordering input.
func handleRoadmapAddV2(db *sql.DB, in roadmapInput) (*mcpx.CallToolResult, any, error) {
	section := roadmap.Section(in.Section)
	if !roadmap.ValidSection(section) {
		return mcpx.ErrorResult(fmt.Sprintf("invalid section %q: must be %s", section, validSectionsMsg)), nil, nil
	}

	var blockersIn []blockerInput
	if in.BlockedBy != nil {
		blockersIn = *in.BlockedBy
	}
	blockers, err := blockersFromInputV2(blockersIn)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	item := roadmap.Item{
		Title:         strOrEmptyV2(in.Title),
		Body:          strOrEmptyV2(in.Body),
		Component:     strOrEmptyV2(in.Component),
		Why:           strOrEmptyV2(in.Why),
		ResumeTrigger: strOrEmptyV2(in.ResumeTrigger),
		KB:            intSliceOrNilV2(in.KB),
		Ticket:        tagsOrNilV2(in.Ticket),
		BlockedBy:     blockers,
		Epic:          strOrEmptyV2(in.Epic),
	}

	var position []int
	if in.Position != nil {
		position = []int{int(*in.Position)}
	}

	var id int
	mutateErr := roadmap.Mutate(db, in.Project, func(rm *roadmap.Roadmap) error {
		var err error
		id, err = roadmap.AddItem(rm, section, item, position...)
		return err
	})
	if mutateErr != nil {
		return mcpx.ErrorResult(mutateErr.Error()), nil, nil
	}

	return jsonResultV2(map[string]interface{}{"id": id, "section": string(section)})
}

// handleRoadmapUpdateV2 mirrors handleRoadmapUpdate: requires id, builds a
// roadmap.Patch straight from roadmapInput's pointer fields (nil = untouched,
// non-nil (even empty) = clears), and applies it via roadmap.UpdateItem
// inside roadmap.Mutate.
func handleRoadmapUpdateV2(db *sql.DB, in roadmapInput) (*mcpx.CallToolResult, any, error) {
	if in.ID == nil {
		return mcpx.ErrorResult(errRoadmapIDRequired), nil, nil
	}

	var blockedBy *[]roadmap.Blocker
	if in.BlockedBy != nil {
		v, err := blockersFromInputV2(*in.BlockedBy)
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}
		blockedBy = &v
	}

	patch := roadmap.Patch{
		Title:         in.Title,
		Body:          in.Body,
		Component:     in.Component,
		Why:           in.Why,
		ResumeTrigger: in.ResumeTrigger,
		KB:            in.KB,
		Ticket:        in.Ticket,
		BlockedBy:     blockedBy,
		Epic:          in.Epic,
	}

	id := *in.ID
	mutateErr := roadmap.Mutate(db, in.Project, func(rm *roadmap.Roadmap) error {
		return roadmap.UpdateItem(rm, id, patch)
	})
	if mutateErr != nil {
		return mcpx.ErrorResult(mutateErr.Error()), nil, nil
	}

	return jsonResultV2(map[string]interface{}{"id": id})
}

// handleRoadmapMoveV2 mirrors handleRoadmapMove: requires id and a valid
// target section (to); position is optional and presence-signaled.
func handleRoadmapMoveV2(db *sql.DB, in roadmapInput) (*mcpx.CallToolResult, any, error) {
	if in.ID == nil {
		return mcpx.ErrorResult(errRoadmapIDRequired), nil, nil
	}

	to := roadmap.Section(in.To)
	if !roadmap.ValidSection(to) {
		return mcpx.ErrorResult(fmt.Sprintf("invalid section %q: must be %s", to, validSectionsMsg)), nil, nil
	}

	var position []int
	if in.Position != nil {
		position = []int{int(*in.Position)}
	}

	id := *in.ID
	mutateErr := roadmap.Mutate(db, in.Project, func(rm *roadmap.Roadmap) error {
		return roadmap.MoveItem(rm, id, to, position...)
	})
	if mutateErr != nil {
		return mcpx.ErrorResult(mutateErr.Error()), nil, nil
	}

	return jsonResultV2(map[string]interface{}{"id": id, "section": string(to)})
}

// handleRoadmapReorderV2 mirrors handleRoadmapReorder: requires both id and
// position (unlike add/move, reorder's position is not optional).
func handleRoadmapReorderV2(db *sql.DB, in roadmapInput) (*mcpx.CallToolResult, any, error) {
	if in.ID == nil {
		return mcpx.ErrorResult(errRoadmapIDRequired), nil, nil
	}
	if in.Position == nil {
		return mcpx.ErrorResult(errRoadmapPositionRequired), nil, nil
	}

	id := *in.ID
	position := int(*in.Position)
	mutateErr := roadmap.Mutate(db, in.Project, func(rm *roadmap.Roadmap) error {
		return roadmap.ReorderItem(rm, id, position)
	})
	if mutateErr != nil {
		return mcpx.ErrorResult(mutateErr.Error()), nil, nil
	}

	return jsonResultV2(map[string]interface{}{"id": id, "position": position})
}

// handleRoadmapDoneV2 mirrors handleRoadmapDone: requires id, moves the item
// to the done section via roadmap.MarkDone.
func handleRoadmapDoneV2(db *sql.DB, in roadmapInput) (*mcpx.CallToolResult, any, error) {
	if in.ID == nil {
		return mcpx.ErrorResult(errRoadmapIDRequired), nil, nil
	}

	id := *in.ID
	mutateErr := roadmap.Mutate(db, in.Project, func(rm *roadmap.Roadmap) error {
		return roadmap.MarkDone(rm, id)
	})
	if mutateErr != nil {
		return mcpx.ErrorResult(mutateErr.Error()), nil, nil
	}

	return jsonResultV2(map[string]interface{}{"id": id, "section": string(roadmap.SectionDone)})
}

// handleRoadmapRemoveV2 mirrors handleRoadmapRemove: requires id, deletes
// the item entirely via roadmap.RemoveItem.
func handleRoadmapRemoveV2(db *sql.DB, in roadmapInput) (*mcpx.CallToolResult, any, error) {
	if in.ID == nil {
		return mcpx.ErrorResult(errRoadmapIDRequired), nil, nil
	}

	id := *in.ID
	mutateErr := roadmap.Mutate(db, in.Project, func(rm *roadmap.Roadmap) error {
		return roadmap.RemoveItem(rm, id)
	})
	if mutateErr != nil {
		return mcpx.ErrorResult(mutateErr.Error()), nil, nil
	}

	return jsonResultV2(map[string]interface{}{"id": id, "removed": true})
}
