package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

func TestAdd(t *testing.T) {
	if 10 + 100 != 110 {
		t.Fail()
	}
}