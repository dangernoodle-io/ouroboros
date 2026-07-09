package roadmap

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RenderMarkdown renders rm as a scannable Markdown mirror, in section
// render order: In flight (now) / Next / Deferred / Parked / Dropped /
// Shipped (done). by selects the grouping axis: "component" (default) or
// "epic" — every item's value on the OTHER axis renders as an inline chip.
// epicLabels resolves an epic item's id to a display title (falls back to
// the raw id when absent or nil); blockedEpics marks epic ids whose epic
// item is itself blocked, rendered as a ⛔ on that epic's group header (nil
// means none blocked). Section slices are already in canonical Position
// order (see canonicalizeSections, applied on every Save); RenderMarkdown
// groups within that order via groupByAxis, which does not re-sort. This is
// the render the /roadmap skill hands to the Artifact tool.
func RenderMarkdown(rm *Roadmap, by string, epicLabels map[string]string, blockedEpics map[string]bool) string {
	axis := normalizeAxis(by)
	var b strings.Builder
	b.WriteString("# Roadmap\n\n")
	renderSection(&b, "In flight", rm.Sections.Now, false, axis, epicLabels, blockedEpics)
	renderSection(&b, "Next", rm.Sections.Next, false, axis, epicLabels, blockedEpics)
	renderSection(&b, "Deferred", rm.Sections.Deferred, true, axis, epicLabels, blockedEpics)
	renderSection(&b, "Parked", rm.Sections.Parked, true, axis, epicLabels, blockedEpics)
	renderSection(&b, "Dropped", rm.Sections.Dropped, false, axis, epicLabels, blockedEpics)
	renderSection(&b, "Shipped", rm.Sections.Done, false, axis, epicLabels, blockedEpics)
	return b.String()
}

// normalizeAxis maps by to a valid grouping axis: the literal "epic", or
// "component" for anything else (including the empty string).
func normalizeAxis(by string) string {
	if by == "epic" {
		return "epic"
	}
	return "component"
}

// axisValue returns it's value along axis ("component" or "epic").
func axisValue(it Item, axis string) string {
	if axis == "epic" {
		return it.Epic
	}
	return it.Component
}

// axisGroup is one grouping-axis bucket within a section: every item
// sharing the same axis value (or the ungrouped "" bucket).
type axisGroup struct {
	value string
	items []Item
}

// groupByAxis partitions items into axis-value buckets, in canonical group
// order: the ungrouped bucket first (if non-empty), then each named value
// in first-appearance order. Item order within each bucket is preserved
// (sections are already Position-sorted by canonicalizeSections on Save).
func groupByAxis(items []Item, axis string) []axisGroup {
	order := []string{}
	buckets := map[string][]Item{}
	for _, it := range items {
		v := axisValue(it, axis)
		if _, ok := buckets[v]; !ok {
			order = append(order, v)
		}
		buckets[v] = append(buckets[v], it)
	}

	groups := make([]axisGroup, 0, len(order))
	if unlaned, ok := buckets[""]; ok {
		groups = append(groups, axisGroup{items: unlaned})
	}
	for _, v := range order {
		if v == "" {
			continue
		}
		groups = append(groups, axisGroup{value: v, items: buckets[v]})
	}
	return groups
}

func renderSection(b *strings.Builder, heading string, items []Item, parked bool, axis string, epicLabels map[string]string, blockedEpics map[string]bool) {
	b.WriteString("## " + heading + "\n\n")
	if len(items) == 0 {
		b.WriteString("_none_\n\n")
		return
	}

	for _, g := range groupByAxis(items, axis) {
		if g.value != "" {
			renderGroupHeader(b, axis, g.value, epicLabels, blockedEpics)
		}
		for _, it := range g.items {
			renderItem(b, it, parked, axis, epicLabels)
		}
	}
}

// renderGroupHeader emits the group heading for a non-empty axis value: a
// component group renders as "component: X"; an epic group renders as
// "epic: <label>" (falling back to the raw id when unresolved), with a ⛔
// suffix when blockedEpics marks that epic id blocked.
func renderGroupHeader(b *strings.Builder, axis, value string, epicLabels map[string]string, blockedEpics map[string]bool) {
	if axis == "epic" {
		label := epicLabels[value]
		if label == "" {
			label = value
		}
		marker := ""
		if blockedEpics[value] {
			marker = " ⛔"
		}
		fmt.Fprintf(b, "#### epic: %s%s\n\n", label, marker)
		return
	}
	fmt.Fprintf(b, "#### component: %s\n\n", value)
}

// sortByPosition stable-sorts items by Position ascending; ties (including
// the common case of every item at the unset 0 value) keep their original
// relative order.
func sortByPosition(items []Item) []Item {
	sorted := make([]Item, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Position < sorted[j].Position
	})
	return sorted
}

func renderItem(b *strings.Builder, it Item, parked bool, axis string, epicLabels map[string]string) {
	fmt.Fprintf(b, "### %s\n", it.Title)
	if chip := otherAxisChip(it, axis, epicLabels); chip != "" {
		b.WriteString(chip + "\n")
	}
	if it.Body != "" {
		b.WriteString(it.Body + "\n")
	}
	if parked {
		if it.Why != "" {
			fmt.Fprintf(b, "- Why: %s\n", it.Why)
		}
		if it.ResumeTrigger != "" {
			fmt.Fprintf(b, "- Resume trigger: %s\n", it.ResumeTrigger)
		}
	}
	if len(it.KB) > 0 {
		fmt.Fprintf(b, "kb: %s\n", joinInts(it.KB))
	}
	if len(it.Ticket) > 0 {
		fmt.Fprintf(b, "ticket: %s\n", strings.Join(it.Ticket, ", "))
	}
	for _, bl := range it.BlockedBy {
		line := fmt.Sprintf("⛔ blocked by %s:%s", bl.Project, bl.Ref)
		if bl.Note != "" {
			line += " — " + bl.Note
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
}

// otherAxisChip renders the axis NOT selected for grouping as an inline
// chip on the item, or "" if that item carries no value on the other axis.
// An epic chip shows the resolved label (epicLabels[it.Epic], falling back
// to the raw id when unresolved) — the same label groupByAxis/
// renderGroupHeader uses for the epic-grouped header, so a component chip
// and an epic group header never disagree on how an epic is displayed.
func otherAxisChip(it Item, axis string, epicLabels map[string]string) string {
	if axis == "epic" {
		if it.Component != "" {
			return fmt.Sprintf("`component: %s`", it.Component)
		}
		return ""
	}
	if it.Epic != "" {
		label := epicLabels[it.Epic]
		if label == "" {
			label = it.Epic
		}
		return fmt.Sprintf("`epic: %s`", label)
	}
	return ""
}

func joinInts(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ", ")
}
