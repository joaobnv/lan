package sne

// Copyright (c) 2025, João Breno. See the license.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestToString(t *testing.T) {
	for i := range 1001 {
		if s := ToString(i); s != fmt.Sprint(i) {
			t.Errorf("ToString(%d) = %q, want %q", i, s, fmt.Sprint(i))
		}
	}
}

func TestToStringBin(t *testing.T) {
	for i := range 1001 {
		if s := ToStringBin(i); s != strconv.FormatInt(int64(i), 2) {
			t.Errorf("ToStringBin(%d) = %q, want %q", i, s, strconv.FormatInt(int64(i), 2))
		}
	}
	if s := ToStringBin(-1); s != "invalid" {
		t.Errorf("ToStringBin(%d) = %q, want %q", -1, s, fmt.Sprint(-1))
	}
}

func TestToStringOct(t *testing.T) {
	for i := range 1001 {
		if s := ToStringOct(i); s != strconv.FormatInt(int64(i), 8) {
			t.Errorf("ToStringOct(%d) = %q, want %q", i, s, strconv.FormatInt(int64(i), 8))
		}
	}
	if s := ToStringOct(-1); s != "invalid" {
		t.Errorf("ToStringOct(%d) = %q, want %q", -1, s, fmt.Sprint(-1))
	}
}

func TestToStringHex(t *testing.T) {
	for i := range 1001 {
		if s := ToStringHex(i); s != strings.ToUpper(strconv.FormatInt(int64(i), 16)) {
			t.Errorf("ToStringHex(%d) = %q, want %q", i, s, strings.ToUpper(strconv.FormatInt(int64(i), 16)))
		}
	}
}
