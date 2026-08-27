package fws2

import (
	"fmt"
	"testing"
)

// Copyright (c) 2026, João Breno. See the license.

func FuzzPsfIo(f *testing.PB) {
	a, b := 1, 2
	if s := Sub(a, b); s != a-b {
		fmt.Printf("Sub(%d, %d) = %d, want %d", a, b, s, a-b)
	}
}
