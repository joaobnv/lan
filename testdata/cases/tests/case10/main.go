package main

// Copyright (c) 2026, João Breno. See the license.

import "time"

func Add[T ~int](n ...T) T {
	time.Sleep(10*time.Second)
	var r T
	for _, v := range n {
		r += v
	}
	return r
}

func Sub[T ~int](a T, n ...T) T {
	for i := range n {
		n[i] = -n[i]
	}
	n = append(n, a)
	return Add(n...)
}