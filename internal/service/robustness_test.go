package service

import (
	"math/rand"
	"testing"
	"time"
)

func TestManySeedsRobustness(t *testing.T) {
	store := &fakePlayerStore{players: realDatasetSample()}
	failures := 0
	start := time.Now()
	for seed := int64(0); seed < 200; seed++ {
		engine := NewGameEngine(store, rand.New(rand.NewSource(seed)))
		_, err := engine.GenerateBoard("test")
		if err != nil {
			failures++
		}
	}
	elapsed := time.Since(start)
	t.Logf("fallos en 200 seeds: %d | tiempo total: %v | promedio: %v", failures, elapsed, elapsed/200)
	if failures > 0 {
		t.Errorf("se esperaban 0 fallos con backtracking real, hubo %d", failures)
	}
}
