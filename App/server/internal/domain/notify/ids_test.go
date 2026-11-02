package notify

import "testing"

func TestStableIDDeterministic(t *testing.T) {
	if a, b := StableID("exam:abc-123:1440"), StableID("exam:abc-123:1440"); a != b {
		t.Fatalf("StableID should be deterministic: %d != %d", a, b)
	}
}

func TestStableIDDifferentKeys(t *testing.T) {
	if a, b := StableID("exam:abc-123:1440"), StableID("exam:abc-123:60"); a == b {
		t.Fatalf("different keys should differ: both %d", a)
	}
}

func TestStableIDNonNegative(t *testing.T) {
	for _, key := range []string{"a", "b", "exam:test:1440", "longer-key-here:12345:60"} {
		if id := StableID(key); id < 0 {
			t.Errorf("StableID(%q) = %d, want non-negative", key, id)
		}
	}
}

func TestStableIDKnownValues(t *testing.T) {
	if got, want := StableID(""), int32(69346085); got != want {
		t.Errorf("StableID(\"\") = %d, want %d", got, want)
	}
	if got, want := StableID("a"), int32(100789388); got != want {
		t.Errorf("StableID(\"a\") = %d, want %d", got, want)
	}
	if got, want := StableID("exam:abc-123:1440"), int32(584744424); got != want {
		t.Errorf("StableID(exam key) = %d, want %d", got, want)
	}
}
