// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efivars

import (
	"bytes"
	"testing"
)

func TestNewLoadOptionArgumentFromUTF8(t *testing.T) {
	tests := []struct {
		input string
		want  []byte
		err   error
	}{
		{"ubuntu", []byte{'u', 0, 'b', 0, 'u', 0, 'n', 0, 't', 0, 'u', 0}, nil},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := NewLoadOptionArgumentFromUTF8(tc.input)
			if tc.err != err {
				t.Fatalf("expected error: %v, got: %v", tc.err, err)
			}
			if !bytes.Equal(tc.want, got) {
				t.Fatalf("expected: %v, got: %v", tc.want, got)
			}
		})

	}
}
