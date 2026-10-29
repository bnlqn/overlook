package fetch

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	var c Cache[int]
	reads := 0
	fail := false
	read := func(context.Context) (int, error) {
		reads++
		if fail {
			return 0, errors.New("timeout")
		}
		return reads, nil
	}
	get := func() (int, error) { return c.Get(context.Background(), now, read) }

	if v, err := get(); v != 1 || err != nil {
		t.Fatalf("the first read: %d, %v", v, err)
	}
	now = now.Add(every - time.Second)
	if v, _ := get(); v != 1 || reads != 1 {
		t.Errorf("an answer under ten minutes old is kept: %d after %d reads", v, reads)
	}
	now = now.Add(time.Second)
	if v, _ := get(); v != 2 || reads != 2 {
		t.Errorf("an answer ten minutes old is read again: %d after %d reads", v, reads)
	}
	now = now.Add(every)
	fail = true
	if _, err := get(); err == nil {
		t.Error("a failed read is an error, not the old answer")
	}
	if _, err := get(); err == nil || reads != 4 {
		t.Errorf("a failed read isn't kept: %d reads", reads)
	}
}
