package testsinpkg_test

// Copyright (c) 2025, João Breno. See the license.

import (
	"testing"
	"testsinpkg"
)

func TestSum(t *testing.T) {
	if s := testsinpkg.Sum(10, 20, 30); s != 60 {
		t.Errorf("sum(10, 20, 30) == %v, want 60", s)
	}
}
