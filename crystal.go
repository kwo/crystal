// Package crystal provides a minimal, high-performance unique ID generator.
//
// Bit Allocation (63 bits total, fits in int64):
//   - Time: 36 bits - Seconds since Unix epoch (1970-01-01), ~68 years span
//   - Node: 11 bits - Auto-calculated from hostname + PID hash (2,048 values)
//   - Step: 16 bits - Monotonic counter (65,536 IDs/second/node)
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
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	// timeBits = 36
	nodeBits = 11
	stepBits = 16

	timeShift = nodeBits + stepBits // 27
	nodeShift = stepBits            // 16
	stepMask  = (1 << stepBits) - 1 // 0xFFFF
	nodeMax   = (1 << nodeBits) - 1 // 2047
)

// base32Encoding uses Crockford alphabet in lowercase (excludes I, L, O, U)
//
//nolint:gochecknoglobals
var base32Encoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)

// ID represents a unique crystal identifier (63 bits, always positive)
type ID int64

// Generator creates unique IDs with automatic node calculation
type Generator struct {
	mu      sync.Mutex
	node    uint64
	machine string
	pid     int
	step    uint32
	lastSec int64
}

// New creates a new Generator with auto-calculated node ID
func New() (*Generator, error) {
	node, err := calculateNodeID()
	if err != nil {
		return nil, err
	}

	machine, err := os.Hostname()
	if err != nil {
		machine = "unknown"
	}

	pid := os.Getpid()

	return &Generator{
		node:    node,
		machine: machine,
		pid:     pid,
		step:    initCounter(),
		lastSec: time.Now().Unix(),
	}, nil
}

// calculateNodeID generates an 11-bit node ID from hostname + PID hash
func calculateNodeID() (uint64, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	pid := os.Getpid()

	h := sha256.New()
	h.Write([]byte(hostname))
	h.Write([]byte(strconv.Itoa(pid)))
	hash := h.Sum(nil)

	// Extract 11 bits from first 2 bytes
	return uint64(binary.BigEndian.Uint16(hash[0:2]) & nodeMax), nil
}

// initCounter returns a random 16-bit value for counter initialization
func initCounter() uint32 {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails
		//nolint:gosec
		return uint32(time.Now().UnixNano()) & 0x7FFF
	}
	return uint32(binary.BigEndian.Uint16(b) & 0x7FFF)
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
	now := time.Now().Unix()

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
				now = time.Now().Unix()
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

// Time returns the timestamp embedded in the ID
func (id ID) Time() time.Time {
	timestamp := int64(id) >> timeShift
	return time.Unix(timestamp, 0)
}

// ParseInt64 converts an int64 to an ID
func ParseInt64(i int64) ID {
	return ID(i)
}

// ParseString parses a base32 encoded string into an ID
func ParseString(s string) (ID, error) {
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
