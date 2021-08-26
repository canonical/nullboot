// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"runtime"
	"testing"
)

func TestGetEfiArchitecture(t *testing.T) {
	arch := GetEfiArchitecture()
	if arch == "" {
		t.Fatalf("Unknown architecture: '%s'", runtime.GOARCH)
	}
}
func TestWriteShimFallback(t *testing.T) {
	tests := []struct {
		label string
		input []BootEntry
		want  string
	}{
		{"basic", []BootEntry{{"shimx64.efi", "ubuntu", "", "This is the boot entry for ubuntu"}}, "shimx64.efi,ubuntu,,This is the boot entry for ubuntu\n"},
		{"fwupd", []BootEntry{
			{"shimx64.efi", "ubuntu", "", "This is the boot entry for ubuntu"},
			{"shimx64.efi", "Linux-Firmware-Updater", "\\fwupdx64.efi", "This is the boot entry for Linux-Firmware-Updater"},
		},
			"shimx64.efi,ubuntu,,This is the boot entry for ubuntu\n" +
				"shimx64.efi,Linux-Firmware-Updater,\\fwupdx64.efi,This is the boot entry for Linux-Firmware-Updater\n",
		},
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
