package main

// Copyright (c) 2026, João Breno. See the license.

func Add(n ...byte) int {
	var r int
	for _, v := range n {
		r += int(v)
	}
	return r
}
