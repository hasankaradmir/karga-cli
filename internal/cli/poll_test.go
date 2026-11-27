package cli

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPoll(t *testing.T) {
	ctx := context.Background()
	turns := 0
	if err := poll(ctx, time.Millisecond, func(n int) (bool, error) { turns = n; return n == 3, nil }); err != nil || turns != 3 {
		t.Fatalf("done at turn 3: %d %v", turns, err)
	}
	boom := errors.New("boom")
	if err := poll(ctx, time.Millisecond, func(int) (bool, error) { return false, boom }); !errors.Is(err, boom) {
		t.Fatalf("error: %v", err)
	}
	ended, cancel := context.WithCancel(ctx)
	cancel()
	if err := poll(ended, time.Hour, func(int) (bool, error) { return false, nil }); err != nil {
		t.Fatalf("an ended context is no failure: %v", err)
	}
}
