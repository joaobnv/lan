// This package contains syntax errors to make go test write to its standard error.
package stderr

// Copyright (c) 2025, João Breno. See the license.

func main() {
	return 1 / 0 // syntax error since Go 1.1
}
