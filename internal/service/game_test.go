package service

import (
	"context"
	"errors"
	"testing"

	"myGuy/internal/domain"
)

func newGameServiceForTest() (*GameService, *fakeUserRepo, *fakeLeaderboardRepo, *countingNotifier) {
	users := newFakeUserRepo()
	board := newFakeLeaderboardRepo()
	notifier := &countingNotifier{}
	return NewGameService(users, board, notifier), users, board, notifier
}

func TestPlayAppliesScoreAndMirrorsItToTheLeaderboard(t *testing.T) {
	svc, users, board, notifier := newGameServiceForTest()
	ctx := context.Background()

	if err := users.Create(ctx, "arash", "irrelevant"); err != nil {
		t.Fatalf("seeding user failed: %v", err)
	}

	result, err := svc.Play(ctx, "arash", "cat")
	if err != nil {
		t.Fatalf("play failed: %v", err)
	}

	if result.Delta != 1 {
		t.Errorf("got delta %d, want 1", result.Delta)
	}
	if result.Total != 1 {
		t.Errorf("got total %d, want 1", result.Total)
	}
	if result.Message != "Good human" {
		t.Errorf("got message %q, want %q", result.Message, "Good human")
	}

	// The leaderboard cache must agree with the durable total.
	if got := board.points["arash"]; got != 1 {
		t.Errorf("leaderboard cache has %d points, want 1", got)
	}
	if notifier.count() != 1 {
		t.Errorf("board was signalled %d times, want 1", notifier.count())
	}
}

func TestPlayScoringRules(t *testing.T) {
	for _, tc := range []struct {
		word      string
		wantDelta int32
		wantMsg   string
	}{
		{"cat", 1, "Good human"},
		{"CAT", 1, "Good human"},
		{"  cat  ", 1, "Good human"},
		{"dog", -1, "Bad human"},
		{"", 0, "Silent human"},
		{"cog", 0, "what?"},
		{"dat", 0, "what?"},
		{"banana", 0, "Unintelligible human"},
	} {
		t.Run(tc.word, func(t *testing.T) {
			gotDelta, gotMsg := domain.ScoreWord(tc.word)
			if gotDelta != tc.wantDelta {
				t.Errorf("got delta %d, want %d", gotDelta, tc.wantDelta)
			}
			if gotMsg != tc.wantMsg {
				t.Errorf("got message %q, want %q", gotMsg, tc.wantMsg)
			}
		})
	}
}

func TestPlayStatusWordReportsTotalWithoutChangingIt(t *testing.T) {
	svc, users, _, _ := newGameServiceForTest()
	ctx := context.Background()

	if err := users.Create(ctx, "arash", "irrelevant"); err != nil {
		t.Fatalf("seeding user failed: %v", err)
	}
	if _, err := svc.Play(ctx, "arash", "cat"); err != nil {
		t.Fatalf("play failed: %v", err)
	}

	result, err := svc.Play(ctx, "arash", "status")
	if err != nil {
		t.Fatalf("play failed: %v", err)
	}
	if result.Delta != 0 {
		t.Errorf("status changed the score by %d, want 0", result.Delta)
	}
	if result.Total != 1 {
		t.Errorf("got total %d, want 1", result.Total)
	}
	if result.Message != "Your value as Human: 1" {
		t.Errorf("got message %q, want %q", result.Message, "Your value as Human: 1")
	}
}

func TestPlayFailsForUnknownUser(t *testing.T) {
	svc, _, _, _ := newGameServiceForTest()

	_, err := svc.Play(context.Background(), "ghost", "cat")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("got %v, want ErrUserNotFound", err)
	}
}
