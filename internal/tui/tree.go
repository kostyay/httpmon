package tui

import (
	"sort"
	"time"

	"github.com/kostyay/httpmon/internal/store"
)

// hostGroup groups flows by host for tree view rendering.
type hostGroup struct {
	Host   string
	Flows  []store.FlowMeta
	Newest time.Time
}

// treeRow is a cursor-addressable row in the flattened tree.
type treeRow struct {
	IsHost bool           // true = host node, false = flow row
	Host   string         // host name (always set)
	Flow   store.FlowMeta // only valid when IsHost == false
}

// buildHostGroups groups flows by host, sorted by most recent activity.
func buildHostGroups(flows []store.FlowMeta) []hostGroup {
	byHost := make(map[string]*hostGroup)
	var order []string

	for _, f := range flows {
		g, ok := byHost[f.Host]
		if !ok {
			g = &hostGroup{Host: f.Host}
			byHost[f.Host] = g
			order = append(order, f.Host)
		}
		g.Flows = append(g.Flows, f)
		if f.StartedAt.After(g.Newest) {
			g.Newest = f.StartedAt
		}
	}

	groups := make([]hostGroup, 0, len(byHost))
	for _, h := range order {
		groups = append(groups, *byHost[h])
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Newest.After(groups[j].Newest)
	})

	return groups
}

// flattenTree converts host groups into cursor-addressable rows based on expand state.
func flattenTree(groups []hostGroup, expanded map[string]bool) []treeRow {
	var rows []treeRow
	for _, g := range groups {
		rows = append(rows, treeRow{IsHost: true, Host: g.Host})
		if expanded[g.Host] {
			for _, f := range g.Flows {
				rows = append(rows, treeRow{IsHost: false, Host: g.Host, Flow: f})
			}
		}
	}
	return rows
}

// flattenFocus returns rows for a single focused host (no host node row).
func flattenFocus(groups []hostGroup, host string) []treeRow {
	for _, g := range groups {
		if g.Host == host {
			rows := make([]treeRow, len(g.Flows))
			for i, f := range g.Flows {
				rows[i] = treeRow{IsHost: false, Host: g.Host, Flow: f}
			}
			return rows
		}
	}
	return nil
}
