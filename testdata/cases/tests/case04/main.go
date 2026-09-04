package main

// Copyright (c) 2026, João Breno. See the license.

import "time"

func Mul(n ...byte) int {
	time.Sleep(10 * time.Second)
	var r int = 1
	for _, v := range n {
		r *= int(v)
	}
	return r
}
