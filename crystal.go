// Package crystal provides a minimal, high-performance unique ID generator.
//
// Bit Allocation (63 bits total, fits in int64):
//   - Time: Configurable via Timebits (default 42) - Milliseconds since epoch
//   - Step: Remaining bits (default 21) - Monotonic counter each millisecond
//
// Features:
//   - No configuration required, fully automatic node calculation
//   - Counter starts from random value (not 0) for better distribution
//   - Lock-free atomic operations for thread safety
//   - Base32 encoding for compact string representation
//   - Time.Time extraction from generated IDs
package crystal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	totalBits   = 63
	minTimebits = 40
	maxTimebits = 48
)

// ID represents a unique crystal identifier (63 bits, always positive)
type ID int64

const defaultEpochMillis = int64(1577836800000) // 2020-01-01 00:00:00 UTC

// Package-level overrides applied when creating new generators.
var (
	// Epoch overrides the timestamp base (milliseconds since Unix epoch) when non-zero.
	Epoch int64 = defaultEpochMillis
	// Timebits controls how many bits are assigned to the timestamp (default 42, range 40-48).
	Timebits = 42
	// base32Encoding uses Crockford alphabet in lowercase (excludes I, L, O, U)
	//
	//nolint:gochecknoglobals
	base32Encoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)
)

// Generator creates unique IDs with automatic node calculation
type Generator struct {
	mu         sync.Mutex
	step       uint64
	lastMillis int64
	seed       [32]byte
}

// New creates a new Generator using the current package-level configuration.
func New() *Generator {
	seed := calculateNodeSeed()

	return &Generator{
		seed:       seed,
		step:       initCounter(seed),
		lastMillis: epochMillis(),
	}
}

// Epoch returns the configured epoch as time.Time. When unset it returns the
// Unix epoch (0).
func (g *Generator) Epoch() time.Time {
	sec := Epoch / 1000
	nsec := (Epoch % 1000) * int64(time.Millisecond)
	return time.Unix(sec, nsec).UTC()
}

// Generate creates and returns a unique ID
func (g *Generator) Generate() ID {
	now := epochMillis()

	g.mu.Lock()
	defer g.mu.Unlock()

	mask := currentStepMask()
	shift := currentTimeShift()

	if now < g.lastMillis {
		now = g.lastMillis
	}

	if now == g.lastMillis {
		g.step = (g.step + 1) & mask
		if g.step == 0 {
			for now <= g.lastMillis {
				runtime.Gosched()
				now = epochMillis()
			}
			g.step = initCounter(g.seed)
		}
	} else {
		g.step = initCounter(g.seed)
	}

	g.lastMillis = now

	return ID((uint64(now) << shift) | //nolint:gosec
		(g.step & mask))
}

// Int64 returns the ID as an int64
func (id ID) Int64() int64 {
	return int64(id)
}

// Time returns the timestamp embedded in the ID
func (id ID) Time() time.Time {
	millis := (int64(id) >> currentTimeShift()) + Epoch
	sec := millis / 1000
	nsec := (millis % 1000) * int64(time.Millisecond)
	return time.Unix(sec, nsec)
}

// String returns the base32 encoded string representation
func (id ID) String() string {
	return id.Base32()
}

// Base32 returns the base32 encoded string representation
func (id ID) Base32() string {
	var b [8]byte
	//nolint:gosec
	binary.BigEndian.PutUint64(b[:], uint64(id))
	return base32Encoding.EncodeToString(b[:])
}

// Hex returns the lowercase hexadecimal string representation
func (id ID) Hex() string {
	var b [8]byte
	//nolint:gosec
	binary.BigEndian.PutUint64(b[:], uint64(id))
	return hex.EncodeToString(b[:])
}

// ParseInt64 converts an int64 to an ID
func ParseInt64(i int64) ID {
	return ID(i)
}

// ParseString parses a base32 encoded string into an ID
func ParseString(s string) (ID, error) {
	return ParseBase32(s)
}

// ParseBase32 parses a base32 encoded string into an ID.
func ParseBase32(s string) (ID, error) {
	b, err := base32Encoding.DecodeString(s)
	if err != nil {
		return 0, err
	}
	if len(b) != 8 {
		return 0, base32.CorruptInputError(len(b))
	}
	//nolint:gosec
	return ID(binary.BigEndian.Uint64(b)), nil
}

// ParseHex parses a hexadecimal string into an ID.
func ParseHex(s string) (ID, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return 0, err
	}
	if len(b) != 8 {
		return 0, fmt.Errorf("invalid hex length: %d", len(b))
	}
	//nolint:gosec
	return ID(binary.BigEndian.Uint64(b)), nil
}

// epochMillis returns milliseconds since the configured epoch, clamped to zero
// when the clock drifts backwards.
func epochMillis() int64 {
	millis := time.Now().UnixMilli() - Epoch
	if millis < 0 {
		return 0
	}
	return millis
}

// normalizedTimebits clamps the exported Timebits knob into the supported range
// (40-48 bits) so it always leaves room for at least one sequence bit.
func normalizedTimebits() int {
	t := Timebits
	if t < minTimebits {
		t = minTimebits
	}
	if t > maxTimebits {
		t = maxTimebits
	}
	return t
}

// currentStepBits returns how many bits are currently available for the
// sequence component (total bits minus time bits, with a minimum of one).
func currentStepBits() int {
	bits := totalBits - normalizedTimebits()
	if bits < 1 {
		return 1
	}
	return bits
}

// currentTimeShift converts the current step width into the shift applied when
// packing or unpacking the timestamp.
func currentTimeShift() uint {
	return uint(currentStepBits())
}

// currentStepMask returns a mask that isolates the sequence bits in the ID.
func currentStepMask() uint64 {
	return (uint64(1) << currentTimeShift()) - 1
}

// currentStepSeedMask returns a mask that caps the initial counter seed to the
// lower half of the step's range so we never start near the rollover boundary.
func currentStepSeedMask() uint64 {
	bits := currentStepBits()
	if bits <= 1 {
		return 0
	}
	return (uint64(1) << uint(bits-1)) - 1
}

// calculateNodeSeed derives entropy from the hostname + PID hash, returning the
// full SHA-256 sum for use when seeding the counter.
func calculateNodeSeed() [32]byte {
	machine, err := os.Hostname()
	if err != nil || machine == "" {
		machine = "unknown"
	}

	pid := os.Getpid()

	h := sha256.New()
	h.Write([]byte(machine))
	h.Write([]byte(strconv.Itoa(pid)))
	hash := h.Sum(nil)

	var seed [32]byte
	copy(seed[:], hash)
	return seed
}

// initCounter returns a random seed for the counter. It mixes the 32-byte host
// digest with fresh crypto/rand output, hashes the combination, and then caps
// the result with currentStepSeedMask so the starting position always falls in
// the lower half of the sequence space (avoiding immediate rollover).
func initCounter(seed [32]byte) uint64 {
	mask := currentStepSeedMask()
	if mask == 0 {
		return 0
	}

	var randBuf [32]byte
	if _, err := rand.Read(randBuf[:]); err != nil {
		// Fallback to timestamp if crypto/rand fails
		//nolint:gosec
		fallback := (uint64(time.Now().UnixNano()) ^ binary.BigEndian.Uint64(seed[0:8])) & mask
		return fallback
	}

	h := sha256.New()
	h.Write(seed[:])
	h.Write(randBuf[:])
	sum := h.Sum(nil)
	return binary.BigEndian.Uint64(sum) & mask
}
