package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestRunBacklogListDefault(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical bug", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "Feature", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "PRIORITY")
	assert.Contains(t, output, "AC-1")
	assert.Contains(t, output, "AC-2")
}

func TestRunBacklogListJSON(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Task", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Limit: 20, JSON: true})
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].ID)
	assert.Equal(t, "Task", items[0].Title)
}

func TestRunBacklogListProjectFilter(t *testing.T) {
	db := newTestDB(t)
	proj1, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	proj2, err := backlog.CreateProject(db, "other-project", "OP")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj1.ID, proj1.Prefix, "P0", "AC item", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj2.ID, proj2.Prefix, "P0", "OP item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Projects: []string{"acme-corp"}, Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-1")
	assert.NotContains(t, output, "OP-1")
}

func TestRunBacklogListSingleStatusFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Open", "", "", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Done", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.UpdateItem(db, item2.ID, map[string]string{"status": "done"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Statuses: []string{"done"}, Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-2")
	assert.NotContains(t, output, "AC-1")
}

func TestRunBacklogListPriorityFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "Low", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Priority: "P0", Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Critical")
	assert.NotContains(t, output, "Low")
}

func TestRunBacklogListInvalidPriorityIgnored(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Priority: "bogus", Limit: 20})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Item")
}

func TestRunBacklogListComponentFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Frontend item", "", "", "frontend", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Backend item", "", "", "backend", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Component: "frontend", Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Frontend item")
	assert.NotContains(t, output, "Backend item")
}

func TestRunBacklogListEpicFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "EPIC: Big rollout", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Child item", "", "", "", epic.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Unrelated item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Epic: epic.ID, Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Child item")
	assert.NotContains(t, output, "Unrelated item")
}

func TestRunBacklogListEpicsOnlyFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "EPIC: Big rollout", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Regular item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{EpicsOnly: true, Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "EPIC: Big rollout")
	assert.NotContains(t, output, "Regular item")
}

func TestRunBacklogListSortCreated(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	older, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "First", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2025-01-01T00:00:00Z", older.ID)
	require.NoError(t, err)
	newer, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Second", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2026-01-01T00:00:00Z", newer.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Sort: "created", Limit: 20, JSON: true})
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 2)
	assert.Equal(t, "AC-2", items[0].ID) // newest first
}

func TestRunBacklogListSinceFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{Since: "24h", Limit: 20})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Item")
}

func TestRunBacklogListInvalidSinceErrors(t *testing.T) {
	db := newTestDB(t)
	var buf bytes.Buffer
	err := runBacklogList(&buf, db, backlogListRequestFlags{Since: "not-a-date", Limit: 20})
	require.Error(t, err)
}

func TestRunBacklogListInvalidSortErrors(t *testing.T) {
	db := newTestDB(t)
	var buf bytes.Buffer
	err := runBacklogList(&buf, db, backlogListRequestFlags{Sort: "bogus", Limit: 20})
	require.Error(t, err)
}

// TestRunBacklogListMultiStatusNoStarvation seeds an earlier status
// ("open") with more items than --limit, plus a later status ("done") with
// a higher-priority item, and asserts the done item still surfaces (sorted
// ahead of the lower-priority open items) rather than being starved by a
// naive per-status-limit-then-concat merge.
func TestRunBacklogListMultiStatusNoStarvation(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	// 5 open items, priorities P1..P5 (worse than the done item below).
	for i := 1; i <= 5; i++ {
		_, err := backlog.AddItem(db, proj.ID, proj.Prefix, fmt.Sprintf("P%d", i), fmt.Sprintf("Open %d", i), "", "", "", "")
		require.NoError(t, err)
	}
	// One done item at P0 — better priority than every open item.
	doneItem, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Done best", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.UpdateItem(db, doneItem.ID, map[string]string{"status": "done"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{
		Statuses: []string{"open", "done"},
		Limit:    3,
		JSON:     true,
	})
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 3)
	// The done item (best priority) must survive the trim, sorted first.
	assert.Equal(t, "Done best", items[0].Title)
	assert.Equal(t, "done", items[0].Status)
	// Remaining slots go to the best-priority open items (P1, P2).
	assert.Equal(t, "Open 1", items[1].Title)
	assert.Equal(t, "Open 2", items[2].Title)
}

// TestRunBacklogListMultiStatusDedup guards the merge's id-dedup: the same
// item can't appear twice even if (hypothetically) matched by more than one
// requested status value.
func TestRunBacklogListMultiStatusDedup(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogList(&buf, db, backlogListRequestFlags{
		Statuses: []string{"open", "open"},
		Limit:    20,
		JSON:     true,
	})
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 1)
}

// TestRunBacklogListMultiStatusZeroLimitMatchesSingleStatus locks --limit 0
// to the SAME effective cap (store.ClampLimit's default of 10) for
// multi-status as for single-status, rather than skipping the trim and
// returning up to len(statuses)*perStatusLimit items.
func TestRunBacklogListMultiStatusZeroLimitMatchesSingleStatus(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	// 15 open items — more than the resolved --limit 0 clamp (10).
	for i := 0; i < 15; i++ {
		_, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", fmt.Sprintf("Open %d", i), "", "", "", "")
		require.NoError(t, err)
	}
	// 15 done items — same, in the second status group.
	for i := 0; i < 15; i++ {
		item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", fmt.Sprintf("Done %d", i), "", "", "", "")
		require.NoError(t, err)
		_, err = backlog.UpdateItem(db, item.ID, map[string]string{"status": "done"})
		require.NoError(t, err)
	}

	var singleBuf bytes.Buffer
	err = runBacklogList(&singleBuf, db, backlogListRequestFlags{Statuses: []string{"open"}, Limit: 0, JSON: true})
	require.NoError(t, err)
	var singleItems []backlog.Item
	require.NoError(t, json.Unmarshal(singleBuf.Bytes(), &singleItems))

	var multiBuf bytes.Buffer
	err = runBacklogList(&multiBuf, db, backlogListRequestFlags{Statuses: []string{"open", "done"}, Limit: 0, JSON: true})
	require.NoError(t, err)
	var multiItems []backlog.Item
	require.NoError(t, json.Unmarshal(multiBuf.Bytes(), &multiItems))

	// Single-status --limit 0 resolves to store.ClampLimit's default (10).
	require.Len(t, singleItems, 10)
	// Multi-status --limit 0 must resolve to the SAME effective cap, not
	// skip the trim (which would allow up to 20 here).
	require.Len(t, multiItems, 10)
}

func TestRunBacklogListEmpty(t *testing.T) {
	db := newTestDB(t)
	var buf bytes.Buffer
	err := runBacklogList(&buf, db, backlogListRequestFlags{Limit: 20, JSON: true})
	require.NoError(t, err)
	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	assert.Empty(t, items)
}

// TestBacklogListCmdRunEWiring exercises backlogListCmd's RunE closure
// directly (flag parsing + withDB wiring), proving the cobra registration
// works end-to-end -- runBacklogList's own behavior is already covered above.
func TestBacklogListCmdRunEWiring(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Wired list item", "", "", "", "")
	require.NoError(t, err)

	backlogListProjectFlag = nil
	backlogListStatusFlag = nil
	backlogListPriorityFlag = ""
	backlogListComponentFlag = ""
	backlogListEpicFlag = ""
	backlogListEpicsFlag = false
	backlogListSinceFlag = ""
	backlogListSortFlag = ""
	backlogListLimitFlag = 20
	backlogListJSONFlag = false
	defer func() {
		backlogListProjectFlag = nil
		backlogListStatusFlag = nil
		backlogListPriorityFlag = ""
		backlogListComponentFlag = ""
		backlogListEpicFlag = ""
		backlogListEpicsFlag = false
		backlogListSinceFlag = ""
		backlogListSortFlag = ""
		backlogListLimitFlag = 20
		backlogListJSONFlag = false
	}()

	var buf bytes.Buffer
	backlogListCmd.SetOut(&buf)
	err = backlogListCmd.RunE(backlogListCmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Wired list item")
}
