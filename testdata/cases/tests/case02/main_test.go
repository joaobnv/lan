package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

func init() {
	print("FuzzAdd")
}

func FuzzAdd(f *testing.F) {
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, numbers []byte) {
		if n := Add(numbers...); n != 0 {
			t.Log(n)
			t.Fail()
		}
	})
}

func FuzzAdd1(f *testing.F) {
	f.Add([]byte{2})
	f.Fuzz(func(t *testing.T, numbers []byte) {
		if n := Add(numbers...); n != 2 {
			t.Log(n)
			t.Fail()
		}
	})
}