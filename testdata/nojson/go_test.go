package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"bytes"
	"encoding/json"
	"testing"
)

func Test1(t *testing.T) {
	stdout := new(bytes.Buffer)

	main()
	dec := json.NewDecoder(stdout)
	v := struct{ A string }{}
	err := dec.Decode(&v)

	if err == nil {
		t.Errorf("error == nil")
	}
}
