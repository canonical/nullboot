// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"
	"testing"

	"github.com/canonical/go-efilib"
	"github.com/spf13/afero"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func CheckFilesEqual(fs afero.Fs, want string, got string) error {
	wantBytes, err := afero.ReadFile(fs, want)
	if err != nil {
		return fmt.Errorf("Could not read want: %v", err)
	}
	gotBytes, err := afero.ReadFile(fs, got)
	if err != nil {
		return fmt.Errorf("Could not read got: %v", err)
	}
	if !bytes.Equal(wantBytes, gotBytes) {
		return fmt.Errorf("Expected: %v, got: %v", string(wantBytes), string(gotBytes))
	}
	return nil

}

func TestKernelManagerNewAndInstallKernels(t *testing.T) {
	appArchitecture = "x64"
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	afero.WriteFile(memFs, "/usr/lib/linux/kernel.efi-1.0-12-generic", []byte("1.0-12-generic"), 0644)
	afero.WriteFile(memFs, "/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("1.0-1-generic"), 0644)
	afero.WriteFile(memFs, "/boot/efi/EFI/ubuntu/<dummy>", []byte(""), 0644)
	afero.WriteFile(memFs, "/etc/kernel/cmdline", []byte("root=magic"), 0644)
	afero.WriteFile(memFs, "/boot/efi/EFI/ubuntu/shimx64.efi", []byte("file a"), 0644)
	mockvars := MockEFIVariables{
		map[efi.VariableDescriptor]mockEFIVariable{
			{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{1, 0, 2, 0, 3, 0}, 123},
			{GUID: efi.GlobalVariable, Name: "Boot0001"}:  {UsbrBootCdrom, 42},
		},
	}
	appEFIVars = &mockvars

	// Create an obsolete Boot0000 entry that we want to collect at the end.
	bm, _ := NewBootManagerFromSystem()
	if _, err := bm.FindOrCreateEntry(BootEntry{Filename: "shimx64.efi", Label: "Ubuntu with obsolete kernel", Options: ""}, "/boot/efi/EFI/ubuntu"); err != nil {
		t.Fatal(err)
	}

	km, err := NewKernelManager()
	if err != nil {
		t.Fatalf("Could not create kernel manager: %v", err)
	}
	wantSourceKernels := []string{"kernel.efi-1.0-12-generic", "kernel.efi-1.0-1-generic"}
	if !reflect.DeepEqual(km.sourceKernels, wantSourceKernels) {
		t.Fatalf("Expected %v, got %v", wantSourceKernels, km.sourceKernels)
	}
	var wantTargetKernels []string
	if !reflect.DeepEqual(km.targetKernels, wantTargetKernels) {
		t.Fatalf("Expected %v, got %v", wantTargetKernels, km.targetKernels)
	}

	if err := km.InstallKernels(); err != nil {
		t.Errorf("Could not install kernels: %v", err)
	}

	if err := CheckFilesEqual(memFs, "/usr/lib/linux/kernel.efi-1.0-12-generic", "/boot/efi/EFI/ubuntu/kernel.efi-1.0-12-generic"); err != nil {
		t.Error(err)
	}
	if err := CheckFilesEqual(memFs, "/usr/lib/linux/kernel.efi-1.0-1-generic", "/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic"); err != nil {
		t.Error(err)
	}

	if err := km.CommitToBootLoader(); err != nil {
		t.Errorf("Could not commit to bootloader: %v", err)
	}

	file, err := memFs.Open("/boot/efi/EFI/ubuntu/BOOT" + strings.ToUpper(GetEfiArchitecture()) + ".CSV")
	if err != nil {
		t.Fatalf("Could not open boot.csv: %v", err)
	}
	reader := transform.NewReader(file, unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder())
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatalf("Could not read boot.csv: %v", err)
	}

	want := ("shim" + GetEfiArchitecture() + ".efi,Ubuntu with kernel 1.0-1-generic,\\kernel.efi-1.0-1-generic root=magic,Ubuntu entry for kernel 1.0-1-generic\n" +
		"shim" + GetEfiArchitecture() + ".efi,Ubuntu with kernel 1.0-12-generic,\\kernel.efi-1.0-12-generic root=magic,Ubuntu entry for kernel 1.0-12-generic\n")
	if want != string(data) {
		t.Errorf("Boot entry mismatch:\nExpected:\n%v\nGot:\n%v", want, string(data))
	}

	// Validate we have actually written the EFI stuff we want
	bm, err = NewBootManagerFromSystem()
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}

	// So we already had 1 populated with a foreign boot entry, this should be preserved.
	if !reflect.DeepEqual(bm.bootOrder, []int{2, 3, 1}) {
		t.Fatalf("Unexpected boot order %v", bm.bootOrder)
	}

	for i, desc := range map[int]string{2: "Ubuntu with kernel 1.0-12-generic", 3: "Ubuntu with kernel 1.0-1-generic", 1: "USBR BOOT CDROM"} {
		if bm.entries[i].LoadOption.Desc() != desc {
			t.Errorf("Expected boot entry %d Description %s, got %s", i, desc, bm.entries[i].LoadOption.Desc())
		}

	}
}
func TestKernelManager_noCmdLine(t *testing.T) {
	appArchitecture = "x64"
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	afero.WriteFile(memFs, "/usr/lib/linux/kernel.efi-1.0-12-generic", []byte("1.0-12-generic"), 0644)
	afero.WriteFile(memFs, "/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("1.0-1-generic"), 0644)
	afero.WriteFile(memFs, "/boot/efi/EFI/ubuntu/<dummy>", []byte(""), 0644)
	afero.WriteFile(memFs, "/boot/efi/EFI/ubuntu/shimx64.efi", []byte("file a"), 0644)
	mockvars := MockEFIVariables{
		map[efi.VariableDescriptor]mockEFIVariable{
			{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{1, 0, 2, 0, 3, 0}, 123},
			{GUID: efi.GlobalVariable, Name: "Boot0001"}:  {UsbrBootCdrom, 42},
		},
	}
	appEFIVars = &mockvars

	// Create an obsolete Boot0000 entry that we want to collect at the end.
	bm, _ := NewBootManagerFromSystem()
	if _, err := bm.FindOrCreateEntry(BootEntry{Filename: "shimx64.efi", Label: "Ubuntu with obsolete kernel", Options: ""}, "/boot/efi/EFI/ubuntu"); err != nil {
		t.Fatal(err)
	}

	km, err := NewKernelManager()
	if err := km.InstallKernels(); err != nil {
		t.Errorf("Could not install kernels: %v", err)
	}

	if err := km.CommitToBootLoader(); err != nil {
		t.Errorf("Could not commit to bootloader: %v", err)
	}

	file, err := memFs.Open("/boot/efi/EFI/ubuntu/BOOT" + strings.ToUpper(GetEfiArchitecture()) + ".CSV")
	if err != nil {
		t.Fatalf("Could not open boot.csv: %v", err)
	}
	reader := transform.NewReader(file, unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder())
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatalf("Could not read boot.csv: %v", err)
	}

	want := ("shim" + GetEfiArchitecture() + ".efi,Ubuntu with kernel 1.0-1-generic,\\kernel.efi-1.0-1-generic,Ubuntu entry for kernel 1.0-1-generic\n" +
		"shim" + GetEfiArchitecture() + ".efi,Ubuntu with kernel 1.0-12-generic,\\kernel.efi-1.0-12-generic,Ubuntu entry for kernel 1.0-12-generic\n")
	if want != string(data) {
		t.Errorf("Boot entry mismatch:\nExpected:\n%v\nGot:\n%v", want, string(data))
	}

	// Validate we have actually written the EFI stuff we want
	bm, err = NewBootManagerFromSystem()
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}

	// So we already had 1 populated with a foreign boot entry, this should be preserved.
	if !reflect.DeepEqual(bm.bootOrder, []int{2, 3, 1}) {
		t.Fatalf("Unexpected boot order %v", bm.bootOrder)
	}

	for i, desc := range map[int]string{2: "Ubuntu with kernel 1.0-12-generic", 3: "Ubuntu with kernel 1.0-1-generic", 1: "USBR BOOT CDROM"} {
		if bm.entries[i].LoadOption.Desc() != desc {
			t.Errorf("Expected boot entry %d Description %s, got %s", i, desc, bm.entries[i].LoadOption.Desc())
		}

	}
}

func TestKernelManagerRemoveObsoleteKernels(t *testing.T) {
	appArchitecture = "x64"
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	afero.WriteFile(memFs, "/usr/lib/linux/kernel.efi-1.0-12-generic", []byte("1.0-12-generic"), 0644)
	afero.WriteFile(memFs, "/boot/efi/EFI/ubuntu/kernel.efi-1.0-12-generic", []byte("1.0-12-generic"), 0644)
	afero.WriteFile(memFs, "/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("1.0-1-generic"), 0644)
	afero.WriteFile(memFs, "/boot/efi/EFI/ubuntu/BOOTX64.CSV", []byte(""), 0644)
	afero.WriteFile(memFs, "/etc/kernel/cmdline", []byte("root=magic"), 0644)
	mockvars := MockEFIVariables{
		map[efi.VariableDescriptor]mockEFIVariable{
			{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{}, 123},
		},
	}
	appEFIVars = &mockvars

	km, err := NewKernelManager()
	if err != nil {
		t.Fatalf("Could not create kernel manager: %v", err)
	}
	if err := km.RemoveObsoleteKernels(); err != nil {
		t.Errorf("Failed to remove obsolete kernels: %v", err)
	}

	if _, err := memFs.Stat("/boot/efi/EFI/ubuntu/kernel.efi-1.0-12-generic"); err != nil {
		t.Errorf("missing file: %v\n", err)
	}
	if _, err := memFs.Stat("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic"); err == nil {
		t.Errorf("did not expect obsolete kernel to be present")
	}

	if km.targetKernels != nil {
		t.Errorf("expected list of target kernels to be empty now, got: %v", km.targetKernels)
	}

}
