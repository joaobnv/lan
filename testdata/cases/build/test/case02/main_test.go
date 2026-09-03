package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

func TestAdd(t, t2 *testing.T) {
	if Add(1, 2) != 3 {
		t.Fail()
	}
}
