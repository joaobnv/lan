package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

func TestAdd(t *testing.T) {
	if 2+4 != 6 {
		t.Fail()
	}
}
