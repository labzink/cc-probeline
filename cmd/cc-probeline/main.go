// Command cc-probeline renders the Claude Code status line.
//
// Phase 3.0 stub: drains stdin and prints a placeholder.
// Real rendering arrives with Phase 3.1+ (JSONL parser) and Phase 4 (formatter).
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	_, _ = io.Copy(io.Discard, os.Stdin)
	fmt.Println("TODO")
}
