package service

import (
	"context"
	"sync"
	"testing"

	"orion/internal/audit"
	"orion/internal/flags"
	"orion/internal/store"
)

func TestMemoryFlagWriter_ConcurrentUpdates_NoLostAuditEntries(t *testing.T) {
	s := store.NewMemoryStore()
	a := audit.NewMemoryStore()
	w := NewMemoryFlagWriter(s, a)
	ctx := context.Background()

	_, err := w.Upsert(ctx, flags.Flag{Key: "race_test", Enabled: true, RolloutPct: 0})
	if err != nil {
		t.Fatalf("seed upsert failed: %v", err)
	}

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(pct int) {
			defer wg.Done()
			_, err := w.Upsert(ctx, flags.Flag{Key: "race_test", Enabled: true, RolloutPct: pct})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent upsert error: %v", err)
	}

	entries, err := a.List(ctx, "race_test")
	if err != nil {
		t.Fatalf("list audit failed: %v", err)
	}

	want := n + 1
	if len(entries) != want {
		t.Errorf("got %d audit entries, want %d — possible lost update under concurrency", len(entries), want)
	}

	afterValues := map[int]bool{0: true}
	for _, e := range entries {
		if e.After != nil {
			afterValues[e.After.RolloutPct] = true
		}
	}
	for _, e := range entries {
		if e.Action == audit.ActionCreate {
			continue
		}
		if e.Before == nil {
			t.Errorf("entry for pct=%d has nil Before on a non-create action", e.After.RolloutPct)
			continue
		}
		if !afterValues[e.Before.RolloutPct] {
			t.Errorf("entry's Before.RolloutPct=%d does not match any prior entry's After value — stale read under concurrency", e.Before.RolloutPct)
		}
	}

	final, err := s.Get(ctx, "race_test")
	if err != nil {
		t.Fatalf("final get failed: %v", err)
	}
	if final.RolloutPct < 1 || final.RolloutPct > n {
		t.Errorf("final RolloutPct = %d, expected a value between 1 and %d", final.RolloutPct, n)
	}
}