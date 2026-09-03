package main

// Copyright (c) 2026, João Breno. See the license.

import "testing"

type Number interface {
	~int | ~float64 | ~complex128
}

type Adder struct{}

func (a Adder) Add[T Number](n1, n2 T, rs ...T) T {
	result := n1 + n2
	for _, n := range rs {
		result += n
	}
	return result
}

func TestAdd(t *testing.T) {
	if (Adder{}).Add(1, 2, 3, 4, 5) != 15 {
		t.Fail()
	}
}
