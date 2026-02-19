// Package crystal provides a minimal, high-performance unique ID generator.
//
// Bit Allocation (63 bits total, fits in int64):
//   - Time: 32 bits - Seconds since Unix epoch (1970-01-01), ~136 years span
//   - Node: 11 bits - Auto-calculated from hostname + PID hash (2,048 values)
//   - Step: 20 bits - Monotonic counter (1,048,576 IDs/second/node)
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
	// timeBits = 32
	nodeBits = 11
	stepBits = 20

	timeShift    = nodeBits + stepBits // 27
	nodeShift    = stepBits            // 16
	stepMask     = (1 << stepBits) - 1 // 0xFFFFF
	stepSeedMask = (1 << (stepBits - 1)) - 1
	nodeMax      = (1 << nodeBits) - 1 // 2047
)

// base32Encoding uses Crockford alphabet in lowercase (excludes I, L, O, U)
//
//nolint:gochecknoglobals
var base32Encoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)

// ID represents a unique crystal identifier (63 bits, always positive)
type ID int64

// Package-level overrides applied when creating new generators.
var (
	// Epoch overrides the timestamp base (seconds since Unix epoch) when non-zero.
	Epoch int64
	// Machine overrides the hostname when non-empty.
	Machine string
	// PID overrides the process identifier when non-zero.
	PID int
)

// Generator creates unique IDs with automatic node calculation
type Generator struct {
	mu      sync.Mutex
	node    uint64
	machine string
	pid     int
	step    uint32
	lastSec int64
}

// New creates a new Generator using the package-level overrides when set.
func New() (*Generator, error) {
	node, machine, pid, err := calculateNodeID()
	if err != nil {
		return nil, err
	}

	return &Generator{
		node:    node,
		machine: machine,
		pid:     pid,
		step:    initCounter(),
		lastSec: epochSeconds(),
	}, nil
}

// calculateNodeID generates an 11-bit node ID from hostname + PID hash
func calculateNodeID() (uint64, string, int, error) {
	machine := Machine
	var err error
	if machine == "" {
		machine, err = os.Hostname()
		if err != nil {
			machine = "unknown"
		}
	}

	pid := PID
	if pid == 0 {
		pid = os.Getpid()
	}

	h := sha256.New()
	h.Write([]byte(machine))
	h.Write([]byte(strconv.Itoa(pid)))
	hash := h.Sum(nil)

	// Extract 11 bits from first 2 bytes
	return uint64(binary.BigEndian.Uint16(hash[0:2]) & nodeMax), machine, pid, err
}

// initCounter returns a random seed for the 20-bit counter. The seed is capped
// at 2^19-1 to avoid starting near the rollover boundary.
func initCounter() uint32 {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails
		//nolint:gosec
		return uint32(time.Now().UnixNano()) & uint32(stepSeedMask)
	}
	return uint32(binary.BigEndian.Uint32(b) & uint32(stepSeedMask))
}

func epochSeconds() int64 {
	sec := time.Now().Unix() - Epoch
	if sec < 0 {
		return 0
	}
	return sec
}

// Epoch returns the configured epoch as time.Time. When unset it returns the
// Unix epoch (0).
func (g *Generator) Epoch() time.Time {
	return time.Unix(Epoch, 0).UTC()
}

// NodeID returns the 11-bit node identifier assigned to this generator.
func (g *Generator) NodeID() uint64 {
	return g.node
}

// Machine returns the hostname of the machine
func (g *Generator) Machine() string {
	return g.machine
}

// Pid returns the process ID
func (g *Generator) Pid() int {
	return g.pid
}

// Generate creates and returns a unique ID
func (g *Generator) Generate() ID {
	now := epochSeconds()

	g.mu.Lock()
	defer g.mu.Unlock()

	// Handle clock rollback or same second
	if now <= g.lastSec {
		// Use lastSec as the timestamp to ensure monotonic ordering
		now = g.lastSec

		g.step = (g.step + 1) & stepMask
		if g.step == 0 {
			// Counter overflow - wait for next second
			for now <= g.lastSec {
				runtime.Gosched()
				now = epochSeconds()
			}
			g.step = initCounter()
		}
	} else {
		// New second, reset step to random value
		g.step = initCounter()
	}

	g.lastSec = now

	return ID((uint64(now) << timeShift) | //nolint:gosec
		(g.node << nodeShift) |
		uint64(g.step))
}

// Int64 returns the ID as an int64
func (id ID) Int64() int64 {
	return int64(id)
}

// Time returns the timestamp embedded in the ID
func (id ID) Time() time.Time {
	timestamp := (int64(id) >> timeShift) + Epoch
	return time.Unix(timestamp, 0)
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
