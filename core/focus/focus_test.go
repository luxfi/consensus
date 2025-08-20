package focus

import (
	"testing"
	"time"
)

func TestTracker(t *testing.T) {
	tracker := NewTracker[string]()

	// Test initial state
	if tracker.Count("item1") != 0 {
		t.Error("expected count 0 for new item")
	}

	// Test increment
	tracker.Incr("item1")
	if tracker.Count("item1") != 1 {
		t.Error("expected count 1 after increment")
	}

	// Test multiple increments
	tracker.Incr("item1")
	tracker.Incr("item1")
	if tracker.Count("item1") != 3 {
		t.Error("expected count 3 after 3 increments")
	}

	// Test reset
	tracker.Reset("item1")
	if tracker.Count("item1") != 0 {
		t.Error("expected count 0 after reset")
	}

	// Test multiple items
	tracker.Incr("item2")
	tracker.Incr("item2")
	if tracker.Count("item2") != 2 {
		t.Error("expected count 2 for item2")
	}
	if tracker.Count("item1") != 0 {
		t.Error("item1 should still be 0")
	}
}

func TestConfidence(t *testing.T) {
	conf := NewConfidence[string](3, 0.8)

	// Test initial state
	s, decided := conf.State("item1")
	if s != 0 || decided {
		t.Error("expected state 0 and not decided for new item")
	}

	// Test building confidence with high ratio
	conf.Update("item1", 0.9)
	s, decided = conf.State("item1")
	if s != 1 || decided {
		t.Errorf("expected state 1, not decided after first update, got state=%d decided=%v", s, decided)
	}

	// Continue building confidence
	conf.Update("item1", 0.85)
	s, decided = conf.State("item1")
	if s != 2 || decided {
		t.Errorf("expected state 2, not decided after second update, got state=%d decided=%v", s, decided)
	}

	// Reach threshold
	conf.Update("item1", 0.9)
	s, decided = conf.State("item1")
	if s != 3 || !decided {
		t.Errorf("expected state 3 and decided after reaching threshold, got state=%d decided=%v", s, decided)
	}

	// Test confidence reset with low ratio
	conf.Update("item2", 0.9)
	conf.Update("item2", 0.1) // Below (1 - threshold), should reset
	s, decided = conf.State("item2")
	if s != 0 || decided {
		t.Errorf("expected confidence reset with low ratio, got state=%d", s)
	}
}

func TestWindowedConfidence(t *testing.T) {
	conf := NewWindowed[string](2, 0.8, 100*time.Millisecond)

	// Test within window
	conf.Update("item1", 0.9)
	conf.Update("item1", 0.85)
	s, decided := conf.State("item1")
	if s != 2 || !decided {
		t.Errorf("expected decided within window, got state=%d decided=%v", s, decided)
	}

	// Test window expiry
	conf.Update("item2", 0.9)
	time.Sleep(150 * time.Millisecond)
	s, decided = conf.State("item2")
	if s != 0 || decided {
		t.Error("expected state reset after window expiry")
	}
}

func TestCalc(t *testing.T) {
	// Test with unanimous agreement
	ratio, conf := Calc(10, 10, 0)
	if ratio != 1.0 || conf != 10 {
		t.Errorf("expected ratio=1.0 conf=10 for unanimous, got ratio=%f conf=%d", ratio, conf)
	}

	// Test with majority
	ratio, conf = Calc(8, 10, 0)
	if ratio != 0.8 || conf != 6 {
		t.Errorf("expected ratio=0.8 conf=6 for 8/10, got ratio=%f conf=%d", ratio, conf)
	}

	// Test with exactly half
	ratio, conf = Calc(5, 10, 0)
	if ratio != 0.5 || conf != 0 {
		t.Errorf("expected ratio=0.5 conf=0 for 5/10, got ratio=%f conf=%d", ratio, conf)
	}

	// Test with previous confidence
	_, conf = Calc(9, 10, 5)
	if conf < 8 || conf > 11 {
		t.Errorf("expected conf around 10 with prev=5, got conf=%d", conf)
	}
}

func TestSkipLogic(t *testing.T) {
	// Test conditions for skip detection
	tests := []struct {
		yes, no, unknown int
		shouldSkip       bool
	}{
		{3, 7, 0, true},  // More no votes
		{7, 3, 0, false}, // More yes votes
		{5, 5, 0, false}, // Tie
		{3, 3, 4, false}, // Many unknowns
	}

	for _, test := range tests {
		skip := shouldSkip(test.yes, test.no, test.unknown)
		if skip != test.shouldSkip {
			t.Errorf("yes=%d no=%d unknown=%d: expected skip=%v, got %v",
				test.yes, test.no, test.unknown, test.shouldSkip, skip)
		}
	}
}

func shouldSkip(yes, no, unknown int) bool {
	total := yes + no + unknown
	if total == 0 {
		return false
	}
	return float64(no)/float64(total) > 0.6
}