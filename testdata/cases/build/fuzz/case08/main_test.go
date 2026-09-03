package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type D = testing.T
type E = D
type F = E

func FuzzLenSlice(f *F) {
	if LenSlice([]int{20: 1}) != 21 {
		f.Fail()
	}
}
