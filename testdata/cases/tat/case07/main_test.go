package main

// Copyright (c) 2026, João Breno. See the license.

import (
	"log"
	"testing"
)

func VerifySwap(t *testing.T) {
	if a, b := Swap(1, 2); a != 2 || b != 1 {
		t.Fail()
	}
}

func Testswap() {
	if a, b := Swap(1, 2); a != 2 || b != 1 {
		log.Print("fail")
	}
}
