package testfail

// Copyright (c) 2025, João Breno. See the license.

import "testing"

func TestSum(t *testing.T) {
	if s := sum(10, 20, 30); s != 0 {
		t.Errorf("sum(10, 20, 30) == %v, want 0", s)
	}
}
