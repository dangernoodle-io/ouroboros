package roadmap

import (
	"dangernoodle.io/ouroboros/internal/backlog"
)

// maxSeedBodyBytes caps the truncated backlog description copied onto a
// seeded roadmap item's Body.
const maxSeedBodyBytes = 280

// SeedResult summarizes a Seed call for CLI reporting.
type SeedResult struct {
	Added    int // brand-new roadmap items created
	Skipped  int // already-seeded items left untouched (additive mode)
	Replaced int // already-seeded items removed and re-added (--replace)
}

// Seed buckets backlog items into rm's sections by status/priority and
// copies each item's Component/Epic (the two grouping axes) plus a Ticket
// ref (the backlog item id) onto a new roadmap Item. Call inside a single
// roadmap.Mutate transaction.
//
// Bucketing: status=done -> SectionDone; open P0 -> SectionNow; open P1/P2
// -> SectionNext; open P3+ or unparsed/empty priority -> SectionParked.
//
// Dedup: a backlog item already present on the roadmap (its id appears in
// some existing item's Ticket refs) is, by default, left untouched and
// counted as Skipped — re-running Seed is idempotent. With replace=true,
// that existing roadmap item is removed and a fresh one added instead (so a
// changed priority/status re-buckets it); roadmap items with no matching
// fetched backlog id are never touched.
//
// AddItem/RemoveItem errors are currently unreachable here — bucketSection
// only ever emits a valid Section and findByTicket only ever returns an id
// that exists on rm — but are propagated rather than discarded so a future
// loosening of either doesn't fail silently.
func Seed(rm *Roadmap, items []backlog.Item, replace bool) (SeedResult, error) {
	var res SeedResult
	for _, bi := range items {
		id, found := findByTicket(rm, bi.ID)
		if found {
			if !replace {
				res.Skipped++
				continue
			}
			if err := RemoveItem(rm, id); err != nil {
				return res, err
			}
			if err := addSeedItem(rm, bi); err != nil {
				return res, err
			}
			res.Replaced++
			continue
		}
		if err := addSeedItem(rm, bi); err != nil {
			return res, err
		}
		res.Added++
	}
	return res, nil
}

// addSeedItem creates a roadmap Item from a backlog item and adds it to the
// bucketed section.
func addSeedItem(rm *Roadmap, bi backlog.Item) error {
	item := Item{
		Title:     bi.Title,
		Body:      truncateUTF8(bi.Description, maxSeedBodyBytes),
		Component: bi.Component,
		Epic:      bi.Epic,
		Ticket:    []string{bi.ID},
	}
	_, err := AddItem(rm, bucketSection(bi), item)
	return err
}

// bucketSection maps a backlog item's status/priority to a roadmap section.
func bucketSection(bi backlog.Item) Section {
	if bi.Status == "done" {
		return SectionDone
	}
	n, ok := backlog.ParsePriority(bi.Priority)
	if !ok {
		return SectionParked
	}
	switch n {
	case 0:
		return SectionNow
	case 1, 2:
		return SectionNext
	default:
		return SectionParked
	}
}

// findByTicket returns the id of the roadmap item (in any section) whose
// Ticket refs include ticketID, if any.
func findByTicket(rm *Roadmap, ticketID string) (int, bool) {
	for _, s := range allSections {
		for _, it := range *sectionSlice(rm, s) {
			for _, t := range it.Ticket {
				if t == ticketID {
					return it.ID, true
				}
			}
		}
	}
	return 0, false
}
