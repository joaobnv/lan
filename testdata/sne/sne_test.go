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
	if s := ToString(-1); s != "invalid" {
		t.Errorf("ToString(%d) = %q, want %q", -1, s, fmt.Sprint(-1))
	}
}

func TestToStringHex(t *testing.T) {
	for i := range 1001 {
		if s := ToStringHex(i); s != strings.ToUpper(strconv.FormatInt(int64(i), 16)) {
			t.Errorf("ToStringHex(%d) = %q, want %q", i, s, strings.ToUpper(strconv.FormatInt(int64(i), 16)))
		}
	}
}
