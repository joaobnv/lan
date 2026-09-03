package main_test

// Copyright (c) 2026, João Breno. See the license.

import (
	"math"
	"testing"
)

type Power struct{}

func (p Power) Pow(n1, n2 float64, rs ...float64) float64 {
	result := math.Pow(n1, n2)
	for _, n := range rs {
		result = math.Pow(result, n)
	}
	return result
}

type T = testing.T

func TestPow(t *T) {
	if (Power{}).Pow(2, 3) != 8 {
		t.Fail()
	}
}
