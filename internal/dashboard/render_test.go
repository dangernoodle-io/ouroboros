package dashboard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderHTML_Tile(t *testing.T) {
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Label: "branch", Value: "main", Sub: "clean"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "branch")
	assert.Contains(t, out, "main")
	assert.Contains(t, out, "clean")
}

func TestRenderHTML_Bar_WithMax(t *testing.T) {
	maxVal := 100.0
	frags := []Fragment{
		Bar{V: 1, Type: "bar", Label: "progress", Value: 42, Max: &maxVal, Note: "on track"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "progress")
	assert.Contains(t, out, "width: 42.00%")
	assert.Contains(t, out, "on track")
}

func TestRenderHTML_Bar_NoMax(t *testing.T) {
	frags := []Fragment{
		Bar{V: 1, Type: "bar", Label: "count", Value: 7},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "count")
	assert.Contains(t, out, "bar-value-text")
	assert.Contains(t, out, "7")
	assert.NotContains(t, out, `<div class="bar-fill"`)
}

func TestRenderHTML_Bar_ZeroMax(t *testing.T) {
	zero := 0.0
	frags := []Fragment{
		Bar{V: 1, Type: "bar", Label: "count", Value: 7, Max: &zero},
	}
	assert.NotPanics(t, func() {
		out := RenderHTML("demo", frags, 0)
		assert.Contains(t, out, "bar-value-text")
	})
}

func TestRenderHTML_Group(t *testing.T) {
	frags := []Fragment{
		Group{
			V: 1, Type: "group", Title: "Open items",
			Cards: []Card{
				{Title: "fix bug", Desc: "desc here", State: "open", Ref: "B1-1", Chips: []string{"p1", "backend"}},
			},
		},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "Open items")
	assert.Contains(t, out, "fix bug")
	assert.Contains(t, out, "desc here")
	assert.Contains(t, out, `class="badge"`)
	assert.Contains(t, out, "open")
	assert.Contains(t, out, "p1")
	assert.Contains(t, out, "backend")
	assert.Contains(t, out, "B1-1")
}

func TestRenderHTML_Note(t *testing.T) {
	frags := []Fragment{
		Note{V: 1, Type: "note", Text: "hello world"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "hello world")
	assert.Contains(t, out, `class="note"`)
}

func TestRenderHTML_HTML_Verbatim(t *testing.T) {
	frags := []Fragment{
		HTML{V: 1, Type: "html", HTML: "<b>x</b>"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "<b>x</b>")
}

func TestRenderHTML_SectionGrouping_FirstAppearanceOrder(t *testing.T) {
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Section: "zzz", Label: "a", Value: "1"},
		Tile{V: 1, Type: "tile", Section: "aaa", Label: "b", Value: "2"},
		Tile{V: 1, Type: "tile", Section: "zzz", Label: "c", Value: "3"},
	}
	out := RenderHTML("demo", frags, 0)
	zzzIdx := strings.Index(out, "zzz")
	aaaIdx := strings.Index(out, "aaa")
	assert.Greater(t, zzzIdx, 0)
	assert.Greater(t, aaaIdx, 0)
	assert.Less(t, zzzIdx, aaaIdx)
}

func TestRenderHTML_ProjectSubGrouping_MultiProject(t *testing.T) {
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Section: "git", Label: "a", Value: "1", Project: "proj-b"},
		Tile{V: 1, Type: "tile", Section: "git", Label: "b", Value: "2", Project: "proj-a"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, `class="dash-project"`)
	assert.Contains(t, out, "proj-b")
	assert.Contains(t, out, "proj-a")
	bIdx := strings.Index(out, "proj-b")
	aIdx := strings.Index(out, "proj-a")
	assert.Less(t, bIdx, aIdx)
}

func TestRenderHTML_ProjectSubGrouping_SingleProject_NoHeader(t *testing.T) {
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Section: "git", Label: "a", Value: "1", Project: "proj-a"},
		Tile{V: 1, Type: "tile", Section: "git", Label: "b", Value: "2", Project: "proj-a"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.NotContains(t, out, `class="dash-project"`)
}

func TestRenderHTML_Theme(t *testing.T) {
	out := RenderHTML("demo", nil, 30)
	assert.Contains(t, out, "prefers-color-scheme")
	assert.Contains(t, out, `data-theme="dark"`)
	assert.Contains(t, out, `data-theme="light"`)
	assert.Contains(t, out, `<meta http-equiv="refresh" content="30">`)
}

func TestRenderHTML_NoMetaRefresh_WhenZero(t *testing.T) {
	out := RenderHTML("demo", nil, 0)
	assert.NotContains(t, out, "http-equiv=\"refresh\"")
}

func TestRenderHTML_Empty_NoData(t *testing.T) {
	out := RenderHTML("demo", nil, 0)
	assert.Contains(t, out, "no data")
	assert.Contains(t, out, "<!DOCTYPE html>")
}

func TestRenderHTML_XSS_Escaping(t *testing.T) {
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Label: "<script>alert(1)</script>", Value: "v"},
		HTML{V: 1, Type: "html", HTML: "<b>x</b>"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assert.NotContains(t, out, "<script>alert(1)</script>")
	assert.Contains(t, out, "<b>x</b>")
}

func TestRenderHTML_DefaultSection_NoCrash(t *testing.T) {
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Label: "a", Value: "1"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, defaultSectionLabel)
}

func TestRenderHTML_UnknownAccent_Neutral(t *testing.T) {
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Label: "a", Value: "1", Accent: "bogus"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "tile-neutral")
}

func TestRenderHTML_KnownAccent(t *testing.T) {
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Label: "a", Value: "1", Accent: "good"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "tile-good")
}

func TestRenderHTML_Bar_ClampsPercent(t *testing.T) {
	max100 := 100.0
	frags := []Fragment{
		Bar{V: 1, Type: "bar", Label: "over", Value: 150, Max: &max100},
		Bar{V: 1, Type: "bar", Label: "under", Value: -10, Max: &max100},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "width: 100.00%")
	assert.Contains(t, out, "width: 0.00%")
}

func TestRenderHTML_ProjectSubGrouping_UnattributedFragmentsFirst(t *testing.T) {
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Section: "git", Label: "unattributed", Value: "1"},
		Tile{V: 1, Type: "tile", Section: "git", Label: "a", Value: "1", Project: "proj-a"},
		Tile{V: 1, Type: "tile", Section: "git", Label: "b", Value: "2", Project: "proj-b"},
	}
	out := RenderHTML("demo", frags, 0)
	assert.Contains(t, out, "unattributed")
	unattrIdx := strings.Index(out, "unattributed")
	projHeaderIdx := strings.Index(out, `class="dash-project"`)
	assert.Less(t, unattrIdx, projHeaderIdx)
}
