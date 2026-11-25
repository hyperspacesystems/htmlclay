package main

import (
	"sync/atomic"
	"time"
)

// recoveredEvery is how often the count of recovered changes is read again, so
// a copy kept while the app runs shows up in the menu.
const recoveredEvery = 30 * time.Second

// recoveredCount is the number of recovered changes the menus last saw.
type recoveredCount struct{ n atomic.Int64 }

// refreshRecovered counts the recovered changes and publishes a new snapshot
// when the count changed.
func (a *app) refreshRecovered() {
	if a.rt.versions == nil {
		return
	}
	histories, err := a.rt.versions.ListKept()
	if err != nil {
		a.rt.logger.Printf("Could not count recovered changes: %v", err)
		return
	}
	total := 0
	for _, h := range histories {
		total += len(h.Entries)
	}
	if a.recovered.n.Swap(int64(total)) != int64(total) {
		a.changed()
	}
}

// watchRecovered keeps the count fresh until stop closes.
func (a *app) watchRecovered(stop <-chan struct{}) {
	a.refreshRecovered()
	t := time.NewTicker(recoveredEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			a.refreshRecovered()
		}
	}
}
