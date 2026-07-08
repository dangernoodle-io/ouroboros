package roadmap

import (
	"fmt"
	"strconv"
	"strings"
)

// RenderMarkdown renders rm as a scannable Markdown mirror: Now / Next /
// Parked / Recently done, in that order. This is the render the /roadmap
// skill hands to the Artifact tool.
func RenderMarkdown(rm *Roadmap) string {
	var b strings.Builder
	b.WriteString("# Roadmap\n\n")
	renderSection(&b, "Now", rm.Sections.Now, false)
	renderSection(&b, "Next", rm.Sections.Next, false)
	renderSection(&b, "Parked", rm.Sections.Parked, true)
	renderSection(&b, "Recently done", rm.Sections.Done, false)
	return b.String()
}

func renderSection(b *strings.Builder, heading string, items []Item, parked bool) {
	b.WriteString("## " + heading + "\n\n")
	if len(items) == 0 {
		b.WriteString("_none_\n\n")
		return
	}
	for _, it := range items {
		renderItem(b, it, parked)
	}
}

func renderItem(b *strings.Builder, it Item, parked bool) {
	fmt.Fprintf(b, "### %s\n", it.Title)
	if it.Component != "" {
		fmt.Fprintf(b, "`component: %s`\n", it.Component)
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

func joinInts(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ", ")
}
