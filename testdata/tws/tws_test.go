package ptio

import (
	"fmt"
	"tws/testing"
)

// Copyright (c) 2025, João Breno. See the license.

func TestPstIo(t *testing.T) {
	a, b := 1, 2
	if s := Sub(a, b); s != a-b {
		fmt.Printf("Sub(%d, %d) = %d, want %d", a, b, s, a-b)
	}
}
