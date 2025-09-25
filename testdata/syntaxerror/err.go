// This package contains syntax errors to make go vet write to its standard error.
package syntaxerror

// Copyright (c) 2025, João Breno. See the license.

func main() {
	return 1 / 0
}
