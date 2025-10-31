// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/canonical/go-efilib"
	efi_linux "github.com/canonical/go-efilib/linux"
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
			{GUID: efi.GlobalVariable, Name: "Boot0001"}:  {UsbrBootCdromOptBytes, 42},
		},
	}

	// Create an obsolete Boot0000 entry that we want to collect at the end.
	bm, _ := NewBootManagerForVariables(&mockvars)
	if _, err := bm.FindOrCreateEntry(BootEntry{Filename: "shimx64.efi", Label: "Ubuntu with obsolete kernel", Options: ""}, "/boot/efi/EFI/ubuntu"); err != nil {
		t.Fatal(err)
	}

	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu", &bm)
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

	if err := km.RegisterNewKernelEFIs(); err != nil {
		t.Errorf("could not register new Kernels as EFIs: %v", err)
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

	want := ("shim" + GetEfiArchitecture() + ".efi,Ubuntu with kernel 1.0-1-generic,\\kernel.efi-1.0-1-generic root=magic ,Ubuntu entry for kernel 1.0-1-generic\n" +
		"shim" + GetEfiArchitecture() + ".efi,Ubuntu with kernel 1.0-12-generic,\\kernel.efi-1.0-12-generic root=magic ,Ubuntu entry for kernel 1.0-12-generic\n")
	if want != string(data) {
		t.Errorf("Boot entry mismatch:\nExpected:\n%v\nGot:\n%v", want, string(data))
	}

	// Validate we have actually written the EFI stuff we want
	bm, err = NewBootManagerForVariables(&mockvars)
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}

	// So we already had 1 populated with a foreign boot entry, this should be preserved.
	if !reflect.DeepEqual(bm.bootOrder, []int{2, 3, 1}) {
		t.Fatalf("Unexpected boot order %v", bm.bootOrder)
	}

	for i, desc := range map[int]string{2: "Ubuntu with kernel 1.0-12-generic", 3: "Ubuntu with kernel 1.0-1-generic", 1: "USBR BOOT CDROM"} {
		if bm.entries[i].LoadOption.Description != desc {
			t.Errorf("Expected boot entry %d Description %s, got %s", i, desc, bm.entries[i].LoadOption.Description)
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
			{GUID: efi.GlobalVariable, Name: "Boot0001"}:  {UsbrBootCdromOptBytes, 42},
		},
	}

	// Create an obsolete Boot0000 entry that we want to collect at the end.
	bm, _ := NewBootManagerForVariables(&mockvars)
	if _, err := bm.FindOrCreateEntry(BootEntry{Filename: "shimx64.efi", Label: "Ubuntu with obsolete kernel", Options: ""}, "/boot/efi/EFI/ubuntu"); err != nil {
		t.Fatal(err)
	}

	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu", &bm)
	if err := km.InstallKernels(); err != nil {
		t.Errorf("Could not install kernels: %v", err)
	}

	if err := km.RegisterNewKernelEFIs(); err != nil {
		t.Errorf("could not register new Kernels as EFIs: %v", err)
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

	want := ("shim" + GetEfiArchitecture() + ".efi,Ubuntu with kernel 1.0-1-generic,\\kernel.efi-1.0-1-generic ,Ubuntu entry for kernel 1.0-1-generic\n" +
		"shim" + GetEfiArchitecture() + ".efi,Ubuntu with kernel 1.0-12-generic,\\kernel.efi-1.0-12-generic ,Ubuntu entry for kernel 1.0-12-generic\n")
	if want != string(data) {
		t.Errorf("Boot entry mismatch:\nExpected:\n%v\nGot:\n%v", want, string(data))
	}

	// Validate we have actually written the EFI stuff we want
	bm, err = NewBootManagerForVariables(&mockvars)
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}

	// So we already had 1 populated with a foreign boot entry, this should be preserved.
	if !reflect.DeepEqual(bm.bootOrder, []int{2, 3, 1}) {
		t.Fatalf("Unexpected boot order %v", bm.bootOrder)
	}

	for i, desc := range map[int]string{2: "Ubuntu with kernel 1.0-12-generic", 3: "Ubuntu with kernel 1.0-1-generic", 1: "USBR BOOT CDROM"} {
		if bm.entries[i].LoadOption.Description != desc {
			t.Errorf("Expected boot entry %d Description %s, got %s", i, desc, bm.entries[i].LoadOption.Description)
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
	bm, err := NewBootManagerForVariables(&mockvars)
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}
	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu", &bm)
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

func TestKernelManagerRegisterNewKernelEFIs(t *testing.T) {
	appArchitecture = "x64"
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}

	esp := "/boot/efi"
	targetDir := "/boot/efi/EFI/ubuntu"
	sourceDir := "/usr/lib/linux"

	// Generate fake kernel files
	kernelNames := []string{
		"kernel.efi-1",
		"kernel.efi-2",
		"kernel.efi-6",
		"kernel.efi-3",
		"kernel.efi-5",
	}
	// Maps kernelNames idx to expected bootNum
	expectedBootNumber := []int{4, 3, 0, 2, 1}
	for _, kernelName := range kernelNames {
		kernelSourcePath := path.Join(sourceDir, kernelName)
		kernelTargetPath := path.Join(targetDir, kernelName)
		afero.WriteFile(memFs, kernelSourcePath, []byte(kernelName), 0644)
		afero.WriteFile(memFs, kernelTargetPath, []byte(kernelName), 0644)
	}

	efivars := MockEFIVariables{}
	bm, err := NewBootManagerForVariables(&efivars)
	if err != nil {
		t.Fatalf("unable to create BootManager: %v", err)
	}

	// Pre-generate a couple of EFI variables to test RegisterNewKernelEFIs
	// does not duplicate any of them so we create Boot0000 and Boot0001
	// associated to kernel.efi-1 and kernel.efi-2 to ensure only one of
	// each exist at the end
	preGenNum := 2
	for i := range preGenNum {
		kernelName := kernelNames[i]
		entry := NewKernelBootEntry("Ubuntu", kernelName, "")
		bm.FindOrCreateEntry(entry, targetDir)
	}

	shimPath := path.Join(targetDir, "shimx64.efi")
	afero.WriteFile(memFs, shimPath, []byte("file a"), 0644)

	km, err := NewKernelManager(esp, sourceDir, "ubuntu", &bm)
	if err != nil {
		t.Fatalf("unable to create KernelManager: %v", err)
	}
	if err := km.InstallKernels(); err != nil {
		t.Fatalf("unable to install kernels: %v", err)
	}

	km.RegisterNewKernelEFIs()
	numEntries := len(km.bootManager.entries)
	numKernels := len(kernelNames)
	if numEntries != numKernels {
		t.Errorf("Expected %d entries, got %d", numKernels, numEntries)
	}
	for kernelNameIdx, bootNum := range expectedBootNumber {
		expectedVersionStr := getKernelABI(kernelNames[kernelNameIdx])
		expectedLabel := fmt.Sprintf("Ubuntu with kernel %s", expectedVersionStr)
		gotDescription := km.bootManager.entries[bootNum].LoadOption.Description

		// The "real" comparison is the BootEntryVariable.Data however, the
		// only difference between each variable is the Description field
		// and encoding the loadOption requires much more setup since it
		// requires a BootManager with efivars to create the DevicePath
		if gotDescription != expectedLabel {
			t.Errorf("Expected %s, got %s", expectedLabel, gotDescription)
		}
	}

}

func TestKernelManagerSetLatestKernelToBootNext(t *testing.T) {
	// Setup mocked/global filesystem
	appArchitecture = "x64"
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}

	targetDir := "/boot/efi/EFI/ubuntu"
	sourceDir := "/usr/lib/linux"

	shimPath := path.Join(targetDir, "shimx64.efi")
	afero.WriteFile(memFs, shimPath, []byte("file a"), 0644)

	// Map a number of BootNumbers to a set of kernel versions
	// NOTE: when reading files, "1" < "100", but "20" > "100"
	// This setup ensures that `readKernels` is properly sorting on version
	kernelVersionMap := map[int]string{
		1: "1",
		2: "100", // This will be the latest kernel
		3: "20",
	}

	efivars := MockEFIVariables{}
	bm, err := NewBootManagerForVariables(&efivars)
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}

	dp, err := efivars.NewFileDevicePath(shimPath, efi_linux.ShortFormPathHD)
	if err != nil {
		t.Fatalf("unable to create shim device path: %v", err)
	}
	for bootNumber, kernelVersion := range kernelVersionMap {
		kernelName := fmt.Sprintf("kernel.efi-%s", kernelVersion)
		kernelSourcePath := path.Join(sourceDir, kernelName)
		kernelTargetPath := path.Join(targetDir, kernelName)

		// Must be written to file to be read by NewKernelManager and InstallKernels
		afero.WriteFile(memFs, kernelSourcePath, []byte(kernelName), 0644)
		afero.WriteFile(memFs, kernelTargetPath, []byte(kernelName), 0644)

		// NOTE: create entries this way in the test so boot number can be controlled
		entry := NewKernelBootEntry("Ubuntu", kernelName, "")
		bootEntryVariable, err := NewBootEntryVariable(entry, bootNumber, dp)
		if err != nil {
			t.Fatalf("unable to create boot entry variable for %s: %v", kernelName, err)
		}
		bm.RegisterBootEntryVariable(bootEntryVariable)
	}

	// This reads the kernel files in version order
	km, err := NewKernelManager("/boot/efi", sourceDir, "ubuntu", &bm)
	// Populate km.bootEntries (also in version order)
	km.InstallKernels()

	if err := km.SetLatestKernelToBootNext(); err != nil {
		t.Fatalf("unexpected error setting latest kernel to BootNext: %v", err)
	}

	// Check BootNext is set correctly in BootManager and on mocked system
	expectedInternalBootNext := 2
	expectedSystemBootNext := toEFIBootEntryBytes(expectedInternalBootNext)
	systemBootNext, _, err := km.bootManager.efivars.GetVariable(efi.GlobalVariable, "BootNext")
	if err != nil {
		t.Fatalf("unexpected error getting BootNext: %v", err)
	}

	if !bytes.Equal(systemBootNext, expectedSystemBootNext) {
		t.Errorf("system BootNext is not correct, expected: %v, got: %v", expectedSystemBootNext, systemBootNext)
	}
	if km.bootManager.bootNext != expectedInternalBootNext {
		t.Errorf("internal BootNext is not correct, expected: %v, got: %v", expectedInternalBootNext, km.bootManager.bootNext)
	}
}

func TestKernelManagerIsCurrentBootLatest(t *testing.T) {
	tests := map[string]struct {
		kernelVersionMap    map[int]string // Maps BootNumber to kernel version string
		latestKernelVersion string         // Version string for the latest kernel version
		bootCurrent         int            // boot number for BootCurrent
		expected            bool
		isErr               bool
	}{
		"BootCurrent is latest": {
			kernelVersionMap: map[int]string{
				1: "3",
				3: "0",
			},
			latestKernelVersion: "3",
			bootCurrent:         1,
			expected:            true,
			isErr:               false,
		},
		"BootCurrent is not latest": {
			kernelVersionMap: map[int]string{
				1: "1",
				3: "3",
			},
			latestKernelVersion: "3",
			bootCurrent:         1,
			expected:            false,
			isErr:               false,
		},
		"Latest not in boot entries": {
			kernelVersionMap: map[int]string{
				1: "1",
				3: "0",
			},
			// Note that latestKernelVersion is not in kernelVersionMap
			latestKernelVersion: "10",
			bootCurrent:         1,
			expected:            false,
			isErr:               true,
		},
	}
	for testName, tt := range tests {
		t.Logf("Testing: %s", testName)

		appArchitecture = "x64"
		memFs := afero.NewMemMapFs()
		appFs = MapFS{memFs}

		mockEntries := make(map[int]BootEntryVariable)
		targetDir := "/boot/efi/EFI/ubuntu"
		shimPath := path.Join(targetDir, "shimx64.efi")
		afero.WriteFile(memFs, shimPath, []byte("file a"), 0644)

		efivars := MockEFIVariables{}
		dp, err := efivars.NewFileDevicePath(shimPath, efi_linux.ShortFormPathHD)
		if err != nil {
			t.Fatalf("unable to create shim device path: %v", err)
		}
		var latestKernelEntry *BootEntry
		for bootNum, version := range tt.kernelVersionMap {
			kernelName := fmt.Sprintf("kernel.efi-%s", version)
			entry := NewKernelBootEntry("Ubuntu", kernelName, "")

			bootEntryVariable, err := NewBootEntryVariable(entry, bootNum, dp)
			if err != nil {
				t.Fatalf("unable to create boot entry variable for %s: %v", kernelName, err)
			}
			if version == tt.latestKernelVersion {
				latestKernelEntry = &entry
			}
			mockEntries[bootNum] = bootEntryVariable
		}
		bm := BootManager{
			efivars:     &efivars,
			entries:     mockEntries,
			bootCurrent: tt.bootCurrent,
		}

		bootEntries := []BootEntry{}
		if latestKernelEntry != nil {
			bootEntries = []BootEntry{*latestKernelEntry}
		}
		km := KernelManager{
			targetDir:   targetDir,
			bootEntries: bootEntries,
			bootManager: &bm,
		}
		isCurrentBootLatest, err := km.IsCurrentBootLatest()
		if err != nil {
			if !tt.isErr {
				t.Errorf("%s: unexpected error: %v", testName, err)
			}
		}
		if isCurrentBootLatest != tt.expected {
			t.Errorf("%s: expected %t, got %t", testName, tt.expected, isCurrentBootLatest)
		}
	}
}
