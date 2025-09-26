package staticcheckunusedfunc

// Copyright (c) 2025, João Breno. See the license.

import "testing"

func TestSum(t *testing.T) {
	if s := Sum(10, 20, 30); s != 60 {
		t.Errorf("sum(10, 20, 30) == %v, want 60", s)
	}
}
