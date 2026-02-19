# Crystal

A minimal unique ID generator for Go.

Crystal generates 63-bit unique identifiers optimized for distributed systems.
It combines a timestamp, machine identifier, and sequence number into a single
sortable ID that fits within a signed 64-bit integer. No configuration or
central coordination is required.

## ID Format

The ID as a whole is a 63-bit integer stored in an int64.

- 1 bit is unused, always set to 0 to keep the ID positive.
- 32 bits are used to store a timestamp with second precision, measured from the configurable `crystal.Epoch` (defaults to the Unix epoch).
- 11 bits are used to store a node ID, automatically derived from a hash of the hostname and process ID.
- 20 bits are used to store a sequence number, starting from a random value and incrementing for each ID generated in the same second.

```
+-----------------------------------------------------------------------------+
| 1 Bit Unused | 32 Bit Timestamp |  11 Bit Node ID  |   20 Bit Sequence ID  |
+-----------------------------------------------------------------------------+
```

Using these settings, this allows for 1,048,576 unique IDs to be generated every
second, per node. The 32-bit timestamp provides a range of ~136 years from
whichever epoch you configure (e.g., Unix epoch → approximately year 2106).

### Node ID

The node ID is derived automatically with no configuration required:

```
SHA256(hostname + PID)[0:2] & 0x07FF
```

This provides 2,048 possible node values with natural distribution across the
ID space. Each unique combination of hostname and process ID produces a
different node value.

### Sequence Number

The sequence number starts from a cryptographically random value (not 0) each
time the second changes. This prevents collision patterns when multiple
processes start simultaneously. If you generate enough IDs in the same second
that the sequence would roll over, the generate function will pause until the
next second.

Internally:
- `initCounter` seeds the per-second counter with a random value capped at `2^19 - 1` (falling back to the current timestamp if needed) so it never starts right next to the rollover boundary.
- `step` stores the live counter value on the `Generator` and increments for every ID created within the same second.
- `stepMask` (`0xFFFFF`) keeps the counter constrained to 20 bits and determines when it wraps/pauses for the next second.

### Clock Rollback Protection

If the system clock moves backwards, the generator continues using the last
known timestamp and incrementing the sequence number, ensuring IDs remain
monotonically increasing.

### String Encoding

IDs can be represented as:
- **Base32** (default) - 13 characters using lowercase Crockford alphabet (`0123456789abcdefghjkmnpqrstvwxyz`). Characters `i`, `l`, `o`, `u` are excluded to avoid visual ambiguity.
- **Hex** - 16 lowercase hexadecimal characters.

## Getting Started

### Installing

This assumes you already have a working Go environment, if not please see
[this page](https://golang.org/doc/install) first.

```sh
go get github.com/kwo/crystal
```

### Usage

Import the package into your project then construct a new crystal Generator.
With the generator call the Generate() method to generate and return a unique
crystal ID.

**Example Program:**

```go
package main

import (
    "fmt"
    "log"

    "github.com/kwo/crystal"
)

func main() {
	// Create a new generator (uses automatic detection by default)
	gen, err := crystal.New()
    if err != nil {
        log.Fatal(err)
    }

    // Generate a crystal ID
    id := gen.Generate()

    // Print out the ID in a few different ways.
    fmt.Printf("Int64  ID: %d\n", id.Int64())
    fmt.Printf("String ID: %s\n", id)
    fmt.Printf("Base32 ID: %s\n", id.Base32())
    fmt.Printf("Hex    ID: %s\n", id.Hex())

    // Print out the ID's timestamp
    fmt.Printf("ID Time  : %v\n", id.Time())

    // Print out the generator's config info
    fmt.Printf("Epoch    : %v\n", gen.Epoch())
    fmt.Printf("Node ID  : %d\n", gen.NodeID())
    fmt.Printf("Machine  : %s\n", gen.Machine())
    fmt.Printf("PID      : %d\n", gen.Pid())
}
```

To override the automatically detected values, set the package-level variables
before calling `New()`:

```go
crystal.Epoch = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
crystal.Machine = "worker-01"
crystal.PID = 4242

gen, err := crystal.New()
```

#### Package-Level Overrides

- `crystal.Epoch` overrides the timestamp base (seconds since Unix epoch). IDs store seconds since this value, and `ID.Time()` adds it back so outputs stay in real time.
- `crystal.Machine` overrides the reported hostname when non-empty.
- `crystal.PID` overrides the reported process ID when non-zero.

### Parsing

```go
// From base32 string
id, err := crystal.ParseString("0d6av3w2kc002")
if err != nil {
    log.Fatal(err)
}

// ParseString is an alias for ParseBase32
id, err := crystal.ParseBase32("0d6av3w2kc002")
if err != nil {
    log.Fatal(err)
}

// From hex string
id, err := crystal.ParseHex("00ff11aa22bb33cc")
if err != nil {
    log.Fatal(err)
}

// From int64
id := crystal.ParseInt64(237755712226918401)
```

### Performance

To benchmark the generator on your system run the following command inside the
crystal package directory.

```sh
go test -run=^$ -bench=.
```

### Comparison

| Feature | Crystal | [xid](https://github.com/rs/xid) | [Snowflake](https://github.com/bwmarrin/snowflake) |
|---------|---------|-----|-----------|
| Bits | 63 | 96 | 63 |
| String Size | 13 chars | 20 chars | up to 20 chars |
| Time Precision | 1 second | 1 second | 1 ms |
| Node Bits | 11 | 40 (24+16) | 10 |
| Sequence Bits | 20 | 24 | 12 |
| Configuration | None | None | Required |
| Sortable | Yes | Yes | Yes |

## License

MIT License - See [LICENSE](LICENSE) file for details.
