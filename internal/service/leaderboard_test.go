package service

import (
	"context"
	"testing"
)

func newLeaderboardServiceForTest() (*LeaderboardService, *fakeUserRepo, *fakeLeaderboardRepo) {
	users := newFakeUserRepo()
	board := newFakeLeaderboardRepo()
	return NewLeaderboardService(users, board, &countingNotifier{}), users, board
}

func TestWarmCacheCopiesDurableScoresIntoTheCache(t *testing.T) {
	svc, users, board := newLeaderboardServiceForTest()
	ctx := context.Background()

	for _, name := range []string{"arash", "sara"} {
		if err := users.Create(ctx, name, "irrelevant"); err != nil {
			t.Fatalf("seeding %s failed: %v", name, err)
		}
	}
	if _, err := users.AddPoints(ctx, "arash", 5); err != nil {
		t.Fatalf("seeding points failed: %v", err)
	}
	if _, err := users.AddPoints(ctx, "sara", 9); err != nil {
		t.Fatalf("seeding points failed: %v", err)
	}

	if err := svc.WarmCache(ctx); err != nil {
		t.Fatalf("warm cache failed: %v", err)
	}

	if got := board.points["arash"]; got != 5 {
		t.Errorf("arash has %d cached points, want 5", got)
	}
	if got := board.points["sara"]; got != 9 {
		t.Errorf("sara has %d cached points, want 9", got)
	}
}

func TestWarmCacheOnEmptyDatabaseIsNotAnError(t *testing.T) {
	svc, _, _ := newLeaderboardServiceForTest()

	if err := svc.WarmCache(context.Background()); err != nil {
		t.Fatalf("warm cache on empty database failed: %v", err)
	}
}

func TestTopRanksByScoreDescending(t *testing.T) {
	svc, _, board := newLeaderboardServiceForTest()
	ctx := context.Background()

	if err := board.SetMany(ctx, map[string]int32{"arash": 5, "sara": 9, "ali": 7}); err != nil {
		t.Fatalf("seeding board failed: %v", err)
	}

	entries, err := svc.Top(ctx, 3)
	if err != nil {
		t.Fatalf("top failed: %v", err)
	}

	want := []struct {
		rank     int32
		username string
		points   int32
	}{
		{1, "sara", 9},
		{2, "ali", 7},
		{3, "arash", 5},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(entries), len(want))
	}
	for i, w := range want {
		got := entries[i]
		if got.Rank != w.rank || got.Username != w.username || got.Points != w.points {
			t.Errorf("entry %d = %+v, want rank=%d username=%s points=%d",
				i, got, w.rank, w.username, w.points)
		}
	}
}

// A caller asking for a silly page size gets the default rather than an
// error or an unbounded query.
func TestTopClampsOutOfRangeSizes(t *testing.T) {
	svc, _, board := newLeaderboardServiceForTest()
	ctx := context.Background()

	seed := make(map[string]int32, 20)
	for i := 0; i < 20; i++ {
		seed[string(rune('a'+i))] = int32(i)
	}
	if err := board.SetMany(ctx, seed); err != nil {
		t.Fatalf("seeding board failed: %v", err)
	}

	for _, topN := range []int32{0, -5, 1000} {
		entries, err := svc.Top(ctx, topN)
		if err != nil {
			t.Fatalf("top(%d) failed: %v", topN, err)
		}
		if len(entries) != defaultTopN {
			t.Errorf("top(%d) returned %d entries, want the default %d", topN, len(entries), defaultTopN)
		}
	}
}
