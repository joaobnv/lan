package main

// Copyright (c) 2026, João Breno. See the license.

func Add[T ~int](n ...T) T {
	var r T
	for _, v := range n {
		r += v
	}
	return r
}
