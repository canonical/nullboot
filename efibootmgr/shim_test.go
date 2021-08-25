// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"testing"
)

func TestWriteShimFallback(t *testing.T) {
	tests := []struct {
		label string
		input []BootEntry
		want  string
	}{
		{"basic", []BootEntry{{"grubx64.efi", "ubuntu", "", "This is the boot entry for Ubuntu"}}, "grubx64.efi,ubuntu,,This is the boot entry for Ubuntu\n"},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			var w bytes.Buffer
			if err := WriteShimFallback(&w, tc.input); err != nil {
				t.Fatalf("error: %v", err)
			}
			got := w.String()
			if tc.want != got {
				t.Fatalf("Output does not match.\nexpected: %v\ngot:\n%v", tc.want, got)
			}
		})

	}
}
