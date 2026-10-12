package api

import (
	"context"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

type Preview struct {
	Name    string    `json:"name"`
	Title   string    `json:"title"`
	Project string    `json:"project"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
	Pinned  bool      `json:"pinned"`
	// Locked is true when the preview needs a password to open.
	Locked bool   `json:"locked"`
	URL    string `json:"url"`
	// Views counts real page opens; LastSeen is the latest one, null if never.
	Views    int64      `json:"views"`
	LastSeen *time.Time `json:"lastSeen"`
	// Comments counts the unresolved comments left from the glim bar.
	Comments int `json:"comments"`
	// Stamp changes on every republish; the push notifier watches it.
	Stamp string `json:"-"`
}

type Status struct {
	Live       int        `json:"live"`
	Pinned     int        `json:"pinned"`
	DiskBytes  int64      `json:"diskBytes"`
	NextExpiry *time.Time `json:"nextExpiry"`
}

type Snapshot struct {
	Now      time.Time `json:"now"`
	Previews []Preview `json:"previews"`
	Status   Status    `json:"status"`
}

func toPreview(st *store.Store, m store.Manifest, v auth.ViewStat) Preview {
	p := Preview{Name: m.Name, Title: m.Title, Project: m.Project, Created: m.Created,
		Expires: m.Expires, Pinned: m.Pinned, Locked: m.Locked(), URL: st.URL(m.Name), Views: v.Count, Stamp: m.Stamp()}
	if v.Count > 0 {
		t := v.LastSeen
		p.LastSeen = &t
	}
	return p
}

// openComments loads every preview's unresolved comment count; like the view
// stats it is decoration, so a nil db or a read error yields none.
func openComments(a *auth.DB) map[string]int {
	if a == nil {
		return nil
	}
	counts, err := a.OpenCommentCounts(context.Background())
	if err != nil {
		return nil
	}
	return counts
}

// viewStats loads every preview's stats; a nil db or a read error yields none,
// since the counter is decoration and must not break the dashboard.
func viewStats(a *auth.DB) map[string]auth.ViewStat {
	if a == nil {
		return nil
	}
	stats, err := a.ViewStats(context.Background())
	if err != nil {
		return nil
	}
	return stats
}

func storeNow(st *store.Store) time.Time {
	if st.Now != nil {
		return st.Now()
	}
	return time.Now()
}

func BuildSnapshot(st *store.Store, a *auth.DB) (Snapshot, error) {
	list, err := st.List()
	if err != nil {
		return Snapshot{}, err
	}
	disk, err := st.DiskUsage()
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Now: storeNow(st).UTC(), Previews: make([]Preview, 0, len(list))}
	snap.Status.Live = len(list)
	snap.Status.DiskBytes = disk
	stats := viewStats(a)
	comments := openComments(a)
	for _, m := range list {
		p := toPreview(st, m, stats[m.Name])
		p.Comments = comments[m.Name]
		snap.Previews = append(snap.Previews, p)
		if m.Pinned {
			snap.Status.Pinned++
			continue
		}
		if snap.Status.NextExpiry == nil || m.Expires.Before(*snap.Status.NextExpiry) {
			e := m.Expires
			snap.Status.NextExpiry = &e
		}
	}
	return snap, nil
}
