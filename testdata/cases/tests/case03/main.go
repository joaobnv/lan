package main

// Copyright (c) 2026, João Breno. See the license.

func Mul(n ...byte) int {
	var r int = 1
	for _, v := range n {
		r *= int(v)
	}
	return r
}