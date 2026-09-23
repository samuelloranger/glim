package api

import (
	"time"

	"github.com/samuelloranger/glim/internal/store"
)

type Preview struct {
	Name    string    `json:"name"`
	Title   string    `json:"title"`
	Project string    `json:"project"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
	Pinned  bool      `json:"pinned"`
	URL     string    `json:"url"`
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

func toPreview(st *store.Store, m store.Manifest) Preview {
	return Preview{Name: m.Name, Title: m.Title, Project: m.Project, Created: m.Created,
		Expires: m.Expires, Pinned: m.Pinned, URL: st.URL(m.Name)}
}

func storeNow(st *store.Store) time.Time {
	if st.Now != nil {
		return st.Now()
	}
	return time.Now()
}

func BuildSnapshot(st *store.Store) (Snapshot, error) {
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
	for _, m := range list {
		snap.Previews = append(snap.Previews, toPreview(st, m))
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
