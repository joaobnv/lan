package to

import "testing"

// Copyright (c) 2025, João Breno. See the license.

func TestSub(t *testing.T) {
	if s := Sub(1, 1); s != 0 {
		t.Errorf("Sub(1, 1) = %d, want 0", s)
	}
}
