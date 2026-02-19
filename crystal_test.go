package crystal

import (
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if gen.Machine() == "" {
		t.Error("Machine() returned empty string")
	}

	if gen.Pid() == 0 {
		t.Error("Pid() returned 0")
	}

	if node := gen.NodeID(); node > nodeMax {
		t.Fatalf("NodeID() out of range: %d", node)
	}
}

func TestGenerate(t *testing.T) {
	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

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
	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

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
	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

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
	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

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
	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

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
	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	id := gen.Generate()
	i := id.Int64()

	parsed := ParseInt64(i)
	if parsed != id {
		t.Errorf("ParseInt64() returned wrong ID: got %d, want %d", parsed.Int64(), id.Int64())
	}
}

func TestInitCounterRange(t *testing.T) {
	for i := 0; i < 1000; i++ {
		val := initCounter()
		if val > uint32(stepSeedMask) {
			t.Fatalf("initCounter returned value out of range: %d", val)
		}
	}
}

func TestPackageLevelOverrides(t *testing.T) {
	origMachine, origPID, origEpoch := Machine, PID, Epoch
	t.Cleanup(func() {
		Machine = origMachine
		PID = origPID
		Epoch = origEpoch
	})

	Machine = "custom-host"
	PID = 4242
	Epoch = 946684800 // 2000-01-01 UTC

	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if gen.machine != Machine {
		t.Fatalf("expected machine %s, got %s", Machine, gen.machine)
	}
	if gen.pid != PID {
		t.Fatalf("expected pid %d, got %d", PID, gen.pid)
	}
	if gen.Epoch().Unix() != Epoch {
		t.Fatalf("expected epoch %d, got %d", Epoch, gen.Epoch().Unix())
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

	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if gen.Epoch().Unix() != 0 {
		t.Fatalf("expected default epoch 0, got %d", gen.Epoch().Unix())
	}
}

func TestConcurrency(t *testing.T) {
	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

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
	gen, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	id := gen.Generate()
	idInt := uint64(id) //nolint:gosec

	// Extract components
	step := idInt & stepMask
	node := (idInt >> nodeShift) & nodeMax
	timestamp := idInt >> timeShift
	realTimestamp := timestamp + uint64(Epoch)

	// Verify step fits in 20 bits
	if step > stepMask {
		t.Errorf("Step exceeds 20 bits: %d", step)
	}

	// Verify node fits in 11 bits
	if node > nodeMax {
		t.Errorf("Node exceeds 11 bits: %d", node)
	}

	// Verify timestamp has reasonable value (not zero, within last minute)
	if realTimestamp == 0 {
		t.Error("Timestamp is zero")
	}

	now := uint64(time.Now().Unix()) //nolint:gosec
	if realTimestamp > now || now-realTimestamp > 60 {
		t.Errorf("Timestamp unreasonable: %d (now: %d)", realTimestamp, now)
	}
}

func BenchmarkGenerate(b *testing.B) {
	gen, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gen.Generate()
	}
}

func BenchmarkGenerateParallel(b *testing.B) {
	gen, err := New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = gen.Generate()
		}
	})
}
