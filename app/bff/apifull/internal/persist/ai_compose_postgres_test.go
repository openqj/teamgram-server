package persist

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestAITonesPostgresMutationIsTransactional(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := OpenPostgres(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ClosePostgres() })
	userID := time.Now().UnixNano()
	t.Cleanup(func() {
		_, _ = MutateAITones(userID, func([]AITone) ([]AITone, error) { return nil, nil })
	})

	const writers = 2
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := MutateAITones(userID, func(current []AITone) ([]AITone, error) {
				return append(current, AITone{ID: int64(len(current) + 1), Title: string(rune('A' + i)), Creator: true}), nil
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	tones, err := LoadAITones(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tones) != writers {
		t.Fatalf("got %d tones after concurrent mutations, want %d", len(tones), writers)
	}
}
