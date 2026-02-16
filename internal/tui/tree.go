package tui

import (
	"sort"
	"time"

	"github.com/kostyay/httpmon/internal/store"
)

// flowGroup groups flows by an arbitrary key for tree view rendering.
type flowGroup struct {
	Key    string
	Flows  []store.FlowMeta
	Newest time.Time
}

// treeRow is a cursor-addressable row in the flattened tree.
type treeRow struct {
	IsHeader bool           // true = group header, false = flow row
	GroupKey string         // group key (always set)
	Flow     store.FlowMeta // only valid when IsHeader == false
}

// Key extractors for grouping.
func hostKey(f store.FlowMeta) string    { return f.Host }
func processKey(f store.FlowMeta) string { return f.Process }

// buildGroups groups flows by keyFn, sorted by most recent activity.
func buildGroups(
	flows []store.FlowMeta,
	keyFn func(store.FlowMeta) string,
) []flowGroup {
	byKey := make(map[string]*flowGroup)
	var order []string

	for _, f := range flows {
		k := keyFn(f)
		g, ok := byKey[k]
		if !ok {
			g = &flowGroup{Key: k}
			byKey[k] = g
			order = append(order, k)
		}
		g.Flows = append(g.Flows, f)
		if f.StartedAt.After(g.Newest) {
			g.Newest = f.StartedAt
		}
	}

	groups := make([]flowGroup, 0, len(byKey))
	for _, k := range order {
		groups = append(groups, *byKey[k])
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Newest.After(groups[j].Newest)
	})

	return groups
}

// flattenTree converts groups into cursor-addressable rows based on expand state.
func flattenTree(groups []flowGroup, expanded map[string]bool) []treeRow {
	var rows []treeRow
	for _, g := range groups {
		rows = append(rows, treeRow{IsHeader: true, GroupKey: g.Key})
		if expanded[g.Key] {
			for _, f := range g.Flows {
				rows = append(rows, treeRow{IsHeader: false, GroupKey: g.Key, Flow: f})
			}
		}
	}
	return rows
}

// flattenFocus returns rows for a single focused group (no header row).
func flattenFocus(groups []flowGroup, key string) []treeRow {
	for _, g := range groups {
		if g.Key == key {
			rows := make([]treeRow, len(g.Flows))
			for i, f := range g.Flows {
				rows[i] = treeRow{IsHeader: false, GroupKey: g.Key, Flow: f}
			}
			return rows
		}
	}
	return nil
}
