# Crystal

A minimal unique ID generator for Go.

Crystal generates 63-bit unique identifiers optimized for distributed systems.
It combines a millisecond timestamp and a seeded sequence counter into a single
sortable ID that fits within a signed 64-bit integer. No configuration or
central coordination is required.

## ID Format

The ID as a whole is a 63-bit integer stored in an int64.

- 1 bit is unused, always set to 0 to keep the ID positive.
- `crystal.Timebits` bits (default 42, configurable between 40 and 48) are used to store a timestamp with millisecond precision, measured from the configurable `crystal.Epoch` (defaults to 2020-01-01T00:00:00Z).
- The remaining bits (default 21) store a sequence number, starting from a seeded random value and incrementing for each ID generated in the same millisecond.

```
+-----------------------------------------------------------------------------+
| 1 Bit Unused | 42 Bit Timestamp |           21 Bit Sequence ID              |
+-----------------------------------------------------------------------------+
```

Using the defaults, each generator can emit 2,097,152 unique IDs every
millisecond. Adjusting `crystal.Timebits` (40–48) trades timestamp range for per-
millisecond throughput (and vice versa). With 42 bits allocated to time you get
~139 years from whichever epoch you configure (e.g., Unix epoch → approx year 2109).

### Seed Material

While the node identifier is no longer embedded in the ID, the hostname+PID
digest (`SHA256(hostname || PID)`) is still computed internally and mixed
directly into the cryptographic RNG that selects the initial sequence value each
millisecond. Separate processes naturally diverge even if they start at the
exact same time.

### Sequence Number

The sequence number starts from a cryptographically random, node-seeded value
each time the millisecond changes. This prevents collision patterns when
multiple processes start simultaneously. If you generate enough IDs in the same
millisecond that the sequence would roll over, the generator waits until the
clock advances.

Internally:
- `initCounter` mixes the hostname/PID hash with cryptographic randomness and caps the starting value at `2^(stepBits-1) - 1` so it never begins right next to the rollover boundary (where `stepBits = 63 - crystal.Timebits`, yielding 15–23 bits of sequence space).
- `step` stores the live counter value on the `Generator` and increments for every ID created within the same millisecond.
- `stepMask` (computed as `(1 << (63 - crystal.Timebits)) - 1`, default `0x1FFFFF`) keeps the counter constrained to the configured number of bits and determines when it wraps/pauses for the next millisecond.

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

Import the package into your project, construct a generator with
`crystal.New()`, and call `Generate()` to return a unique crystal ID.
`New()` automatically determines the host/PID seed for you, while
`crystal.Epoch`/`crystal.Timebits` (40–48) let you tweak the layout before calling `New()`.

**Example Program:**

```go
package main

import (
    "fmt"

    "github.com/kwo/crystal"
)

func main() {
    // Create a new generator (uses automatic detection by default)
    gen := crystal.New()

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
}
```

Override the epoch globally by setting `crystal.Epoch` before constructing the
generator. Adjust `crystal.Timebits` (40–48, also before `New()`) if you need a
different time/sequence split:

```go
crystal.Epoch = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
crystal.Timebits = 40 // optional: 40 time bits, 23 sequence bits

gen := crystal.New()
```

To apply overrides globally, set the package-level variables before calling
`New()`:

```go
crystal.Epoch = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
crystal.Timebits = 44

gen := crystal.New()
```

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
| Time Precision | 1 millisecond | 1 second | 1 ms |
| Node Bits | 0 | 40 (24+16) | 10 |
| Sequence Bits | 21 | 24 | 12 |
| Configuration | None | None | Required |
| Sortable | Yes | Yes | Yes |


## Time Bits

Assuming an unsigned timestamp of milliseconds since 2020-01-01T00:00:00.000Z, the maximum value is 2^bits − 1, so the max date is:

| Bits |      Max value (ms) | Max UTC date/time         | Range (years, months) |
| ---: | ------------------: | ------------------------- | --------------------- |
|   41 |   2,199,023,255,551 | 2089-09-06T15:47:35.551Z  | 69y 8m                |
|   42 |   4,398,046,511,103 | 2159-05-15T07:35:11.103Z  | 139y 4m               |
|   43 |   8,796,093,022,207 | 2298-09-26T15:10:22.207Z  | 278y 8m               |
|   44 |  17,592,186,044,415 | 2577-06-22T06:20:44.415Z  | 557y 5m               |
|   45 |  35,184,372,088,831 | 3134-12-13T12:41:28.831Z  | 1114y 11m             |
|   46 |  70,368,744,177,663 | 4249-11-24T01:22:57.663Z  | 2229y 10m             |
|   47 | 140,737,488,355,327 | 6479-10-17T02:45:55.327Z  | 4459y 9m              |
|   48 | 281,474,976,710,655 | 10939-08-03T05:31:50.655Z | 8919y 7m              |

## License

MIT License - See [LICENSE](LICENSE) file for details.
