package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestCompleteSetupCreatesOnlyTheFirstAccount(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	if _, err := db.CompleteSetup(ctx, "not-an-email", goodPass); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("bad email = %v", err)
	}
	if _, err := db.CompleteSetup(ctx, "sam@example.com", "short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("weak password = %v", err)
	}
	u, err := db.CompleteSetup(ctx, " Sam@Example.com ", goodPass)
	if err != nil || u.Email != "sam@example.com" {
		t.Fatalf("complete = %+v, %v", u, err)
	}
	if _, err := db.CompleteSetup(ctx, "eve@example.com", goodPass); !errors.Is(err, ErrSetupDone) {
		t.Fatalf("second setup = %v", err)
	}
}

func TestConcurrentSetupHasOneWinner(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := db.CompleteSetup(ctx, fmt.Sprintf("user%d@example.com", i), goodPass); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if n, _ := db.CountUsers(ctx); wins != 1 || n != 1 {
		t.Fatalf("wins = %d, users = %d; want 1, 1", wins, n)
	}
}
