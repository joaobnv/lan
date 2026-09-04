package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

func FuzzMul(f *testing.F) {
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, numbers []byte) {
		if n := Mul(numbers...); n != 200 {
			t.Log(n)
			t.Fail()
		}
	})
}

func FuzzMul1(f *testing.F) {
	f.Add([]byte{2})
	f.Fuzz(func(t *testing.T, numbers []byte) {
		if n := Mul(numbers...); n != 2 {
			t.Log(n)
			t.Fail()
		}
	})
}
