package main

import (
	"fmt"
	"log"
	"os"
	"text/tabwriter"
	"time"

	"github.com/kwo/crystal"
)

func main() {
	// Create a new generator
	gen := crystal.New(time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC), "", 0)

	fmt.Printf("Generator initialized:\n")
	fmt.Printf("  Epoch: %s\n", gen.Epoch().Format(time.RFC3339))
	fmt.Printf("  Node ID: %d\n", gen.NodeID())
	fmt.Printf("  Machine: %s\n", gen.Machine())
	fmt.Printf("  PID: %d\n", gen.Pid())
	fmt.Println()

	// Generate some IDs and display in table format
	fmt.Println("Generated IDs:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tInt64\tBase32\tHex\tTime")
	fmt.Fprintln(w, "--\t------------------\t---------------\t----------------\t-------------------")

	for i := 0; i < 10; i++ {
		id := gen.Generate()
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\n",
			i+1,
			id.Int64(),
			id.Base32(),
			id.Hex(),
			id.Time().Format("2006-01-02 15:04:05"))
	}
	w.Flush()

	fmt.Println()

	// Demonstrate parsing from different formats
	id := gen.Generate()
	fmt.Println("Parsing examples:")
	fmt.Printf("  Original ID: %d\n", id.Int64())
	fmt.Printf("  Base32:      %s\n", id.Base32())
	fmt.Printf("  Hex:         %s\n", id.Hex())
	fmt.Println()

	// Parse from base32 string
	idStr := id.String()
	parsed, err := crystal.ParseString(idStr)
	if err != nil {
		log.Fatalf("Failed to parse string: %v", err)
	}

	fmt.Printf("Parsed from Base32:\n")
	fmt.Printf("  Original: %d\n", id.Int64())
	fmt.Printf("  Parsed:   %d\n", parsed.Int64())
	fmt.Printf("  Match:    %v\n", id == parsed)
	fmt.Println()

	// Demonstrate ParseInt64
	intId := id.Int64()
	parsedInt := crystal.ParseInt64(intId)
	fmt.Printf("Parsed from Int64:\n")
	fmt.Printf("  Original: %d\n", id.Int64())
	fmt.Printf("  Parsed:   %d\n", parsedInt.Int64())
	fmt.Printf("  Match:    %v\n", id == parsedInt)
}
