package crystal

import (
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	gen := New()

	if gen.lastMillis < 0 {
		t.Fatal("lastMillis should never be negative")
	}

	var zeroSeed [32]byte
	if gen.seed == zeroSeed {
		t.Fatal("seed was not initialized")
	}
}

func TestGenerate(t *testing.T) {
	gen := New()

	// Generate multiple IDs
	ids := make([]ID, 1000)
	for i := 0; i < 1000; i++ {
		ids[i] = gen.Generate()
	}

	// Check uniqueness
	seen := make(map[ID]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("Duplicate ID generated: %d", id.Int64())
		}
		seen[id] = true
	}

	// Check ordering (IDs should be increasing)
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("IDs not in order: %d <= %d", ids[i], ids[i-1])
		}
	}
}

func TestIDMethods(t *testing.T) {
	gen := New()

	id := gen.Generate()

	// Test Int64
	if id.Int64() == 0 {
		t.Error("Int64() returned 0")
	}

	// Test String
	s := id.String()
	if s == "" {
		t.Error("String() returned empty string")
	}

	// Test Time
	idTime := id.Time()
	if idTime.IsZero() {
		t.Error("Time() returned zero time")
	}

	// ID time should be very close to now
	if time.Since(idTime) > time.Second {
		t.Errorf("ID time too old: %v", idTime)
	}
}

func TestParseString(t *testing.T) {
	gen := New()

	id := gen.Generate()
	s := id.String()

	parsed, err := ParseString(s)
	if err != nil {
		t.Fatalf("ParseString() failed: %v", err)
	}

	if parsed != id {
		t.Errorf("ParseString() returned wrong ID: got %d, want %d", parsed.Int64(), id.Int64())
	}
}

func TestParseBase32(t *testing.T) {
	gen := New()

	id := gen.Generate()
	s := id.Base32()

	parsed, err := ParseBase32(s)
	if err != nil {
		t.Fatalf("ParseBase32() failed: %v", err)
	}

	if parsed != id {
		t.Errorf("ParseBase32() returned wrong ID: got %d, want %d", parsed.Int64(), id.Int64())
	}
}

func TestParseStringInvalid(t *testing.T) {
	_, err := ParseString("invalid!@#")
	if err == nil {
		t.Error("ParseString() should fail for invalid characters")
	}
}

func TestParseBase32Invalid(t *testing.T) {
	_, err := ParseBase32("invalid!@#")
	if err == nil {
		t.Error("ParseBase32() should fail for invalid characters")
	}
}

func TestParseHex(t *testing.T) {
	gen := New()

	id := gen.Generate()
	h := id.Hex()

	parsed, err := ParseHex(h)
	if err != nil {
		t.Fatalf("ParseHex() failed: %v", err)
	}

	if parsed != id {
		t.Errorf("ParseHex() returned wrong ID: got %d, want %d", parsed.Int64(), id.Int64())
	}
}

func TestParseHexInvalid(t *testing.T) {
	_, err := ParseHex("zzzz")
	if err == nil {
		t.Error("ParseHex() should fail for invalid input")
	}
}

func TestParseInt64(t *testing.T) {
	gen := New()

	id := gen.Generate()
	i := id.Int64()

	parsed := ParseInt64(i)
	if parsed != id {
		t.Errorf("ParseInt64() returned wrong ID: got %d, want %d", parsed.Int64(), id.Int64())
	}
}

func TestInitCounterRange(t *testing.T) {
	seed := calculateNodeSeed()
	mask := currentStepSeedMask()
	for i := 0; i < 1000; i++ {
		val := initCounter(seed)
		if mask == 0 {
			if val != 0 {
				t.Fatalf("expected initCounter to return 0 when mask is 0, got %d", val)
			}
			continue
		}
		if val > mask {
			t.Fatalf("initCounter returned value out of range: %d", val)
		}
	}
}

func TestPackageLevelOverrides(t *testing.T) {
	origEpoch := Epoch
	t.Cleanup(func() {
		Epoch = origEpoch
	})

	customEpoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	Epoch = customEpoch

	gen := New()

	if gen.Epoch().UnixMilli() != customEpoch {
		t.Fatalf("expected epoch %d, got %d", customEpoch, gen.Epoch().UnixMilli())
	}

	id := gen.Generate()
	if time.Since(id.Time()) > time.Second {
		t.Fatalf("ID time not near now: %v", id.Time())
	}
}

func TestEpochDefault(t *testing.T) {
	origEpoch := Epoch
	Epoch = 0
	t.Cleanup(func() {
		Epoch = origEpoch
	})

	gen := New()

	if gen.Epoch().UnixMilli() != 0 {
		t.Fatalf("expected default epoch 0, got %d", gen.Epoch().UnixMilli())
	}
}

func TestTimebitsOverride(t *testing.T) {
	origTimebits := Timebits
	origEpoch := Epoch
	t.Cleanup(func() {
		Timebits = origTimebits
		Epoch = origEpoch
	})

	Timebits = 40
	Epoch = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	expectedStepBits := totalBits - normalizedTimebits()
	if expectedStepBits != 23 {
		t.Fatalf("expected step bits 23, got %d", expectedStepBits)
	}

	gen := New()
	id := gen.Generate()

	mask := currentStepMask()
	if mask != (uint64(1)<<uint(expectedStepBits))-1 {
		t.Fatalf("unexpected mask %d", mask)
	}

	if time.Since(id.Time()) > time.Second {
		t.Fatalf("ID time not near now: %v", id.Time())
	}
}

func TestTimebitsClamp(t *testing.T) {
	origTimebits := Timebits
	t.Cleanup(func() {
		Timebits = origTimebits
	})

	Timebits = 100
	if normalizedTimebits() != maxTimebits {
		t.Fatalf("expected clamp to %d, got %d", maxTimebits, normalizedTimebits())
	}

	Timebits = 0
	if normalizedTimebits() != minTimebits {
		t.Fatalf("expected clamp to %d, got %d", minTimebits, normalizedTimebits())
	}
}

func TestConcurrency(t *testing.T) {
	gen := New()

	const numGoroutines = 10
	const idsPerGoroutine = 1000

	var wg sync.WaitGroup
	idChan := make(chan ID, numGoroutines*idsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				idChan <- gen.Generate()
			}
		}()
	}

	wg.Wait()
	close(idChan)

	// Check uniqueness
	seen := make(map[ID]bool)
	for id := range idChan {
		if seen[id] {
			t.Errorf("Duplicate ID in concurrent generation: %d", id.Int64())
		}
		seen[id] = true
	}

	if len(seen) != numGoroutines*idsPerGoroutine {
		t.Errorf("Expected %d unique IDs, got %d", numGoroutines*idsPerGoroutine, len(seen))
	}
}

func TestBitAllocation(t *testing.T) {
	gen := New()

	id := gen.Generate()
	idInt := uint64(id) //nolint:gosec

	mask := currentStepMask()
	shift := currentTimeShift()

	// Extract components
	step := idInt & mask
	timestamp := idInt >> shift
	realMillis := timestamp + uint64(Epoch)

	if step > mask {
		t.Errorf("Step exceeds configured bits: %d", step)
	}

	// Verify timestamp has reasonable value (not zero, within last minute)
	if realMillis == 0 {
		t.Error("Timestamp is zero")
	}

	now := uint64(time.Now().UnixMilli()) //nolint:gosec
	if realMillis > now || now-realMillis > 60_000 {
		t.Errorf("Timestamp unreasonable: %d (now: %d)", realMillis, now)
	}
}

func BenchmarkGenerate(b *testing.B) {
	gen := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gen.Generate()
	}
}

func BenchmarkGenerateParallel(b *testing.B) {
	gen := New()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = gen.Generate()
		}
	})
}
