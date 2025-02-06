// This command is a "go test -json" that generates incorrect json.
package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"fmt"
	"os"
)

// stdout contains the standard output. We use it for allow tests to change the destination of the output.
var stdout = os.Stdout

func main() {
	fmt.Fprint(stdout, "<language>Go<language>")
}
