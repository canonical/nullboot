// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"github.com/spf13/afero"

	"bytes"
	"runtime"
	"testing"
)

func TestGetEfiArchitecture(t *testing.T) {
	appArchitecture = ""
	arch := GetEfiArchitecture()
	if arch == "" {
		t.Fatalf("Unknown architecture: '%s'", runtime.GOARCH)
	}
}
func TestWriteShimFallback(t *testing.T) {
	appArchitecture = "x64"
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

func TestInstallShim_NoKernelsAvailable(t *testing.T) {
	appArchitecture = "x64"
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}

	updated, err := InstallShim("/boot/efi", "/usr/lib/nullboot/shim-signed", "ubuntu")
	if updated {
		t.Errorf("Unexpected update")
	}
	if err == nil {
		t.Errorf("Unexpected success")
	}
}

func TestInstallShim_BasicUpdate(t *testing.T) {
	appArchitecture = "x64"
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}

	afero.WriteFile(memFs, "/usr/lib/nullboot/shim-signed/shimx64.efi.signed", []byte("shim"), 0644)
	afero.WriteFile(memFs, "/usr/lib/nullboot/shim-signed/fbx64.efi", []byte("fb"), 0644)
	afero.WriteFile(memFs, "/usr/lib/nullboot/shim-signed/mmx64.efi", []byte("mm"), 0644)
	afero.WriteFile(memFs, "/boot/efi/EFI/BOOT/BOOTX64.EFI", []byte("old shim"), 0644)

	updated, err := InstallShim("/boot/efi", "/usr/lib/nullboot/shim-signed", "ubuntu")
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if !updated {
		t.Errorf("Expected successful update")
	}

	copies := map[string]string{
		"/boot/efi/EFI/BOOT/BOOTX64.EFI":   "/usr/lib/nullboot/shim-signed/shimx64.efi.signed",
		"/boot/efi/EFI/BOOT/fbx64.efi":     "/usr/lib/nullboot/shim-signed/fbx64.efi",
		"/boot/efi/EFI/BOOT/mmx64.efi":     "/usr/lib/nullboot/shim-signed/mmx64.efi",
		"/boot/efi/EFI/ubuntu/shimx64.efi": "/usr/lib/nullboot/shim-signed/shimx64.efi.signed",
		"/boot/efi/EFI/ubuntu/fbx64.efi":   "/usr/lib/nullboot/shim-signed/fbx64.efi",
		"/boot/efi/EFI/ubuntu/mmx64.efi":   "/usr/lib/nullboot/shim-signed/mmx64.efi",
	}
	for dst, src := range copies {
		if err := CheckFilesEqual(memFs, dst, src); err != nil {
			t.Error(err)
		}
	}
}
