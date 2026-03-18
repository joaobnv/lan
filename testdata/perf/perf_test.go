package des

import (
	"testing"
)

// Copyright (c) 2026, João Breno. See the license.

func TestAdd(t *testing.T) {
	a, b, want := 10, 20, 0
	if got := Add(a, b); got != want {
		t.Errorf("%d + %d = %d, want %d", a, b, got, want)
	}
}
