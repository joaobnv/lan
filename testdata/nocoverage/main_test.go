package main

// Copyright (c) 2025, João Breno. See the license.

import "testing"

func TestSum(t *testing.T) {
	if s := sum[int](); s != 0 {
		t.Errorf("sum() == %v, want 0", s)
	}
}
