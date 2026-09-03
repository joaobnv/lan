package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type R = testing.B
type S = R
type T = S

func TestLenSlice(t *T) {
	if LenSlice([]int{20: 1}) != 21 {
		t.Fail()
	}
}
