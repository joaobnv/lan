package main

// Copyright (c) 2026, João Breno. See the license.

import "strings"

type S[T any] []T
type StringSlice = S[string]

var s StringSlice = []string{"Fuzz", "Add"}

func init() {
	var msg StringSlice
	for i, v := range s {
		msg[i] = v
	}
	print(strings.Join(msg, ""))
}
