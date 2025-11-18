package efibootmgr

import (
	"fmt"

	"github.com/canonical/go-efilib"
	efi_linux "github.com/canonical/go-efilib/linux"
	"github.com/spf13/afero"
)

// MockEnv creates a fully functional mocked filesystem and KernelManager
// to simplify the construction of an environment for testing.
type MockEnv struct {
	memFs afero.Fs
	km    KernelManager
}

// NewMockEnv creates a mocked environment for testing such that the
// installed kernels, boot entries, boot order, and boot current can be
// specifically mapped to validate logic that depends on specific EFI
// variables or files existing at a given point.
// Also allows the construction of invalid states to test failure logic.
//
// Returns a pointer to a MockEnv or nil and an error if the environment
// cannot be initialized.
func NewMockEnv(installedKernels []int, entryBootNums []int, bootOrder []int) (*MockEnv, error) {
	var mockEnv MockEnv

	// Test filesystem
	mockEnv.memFs = *SetupTestFs()

	entryMap := make(map[int]BootEntry)
	entries := make(map[int]BootEntryVariable)
	var bootEntries []BootEntry

	// Generate km.bootEntries
	for _, bootNum := range installedKernels {
		testBootEntry := GenerateTestBootEntry(bootNum)
		entryMap[bootNum] = testBootEntry
		bootEntries = append(bootEntries, testBootEntry)

		// Entry needs to exist in the mock filesystem
		targetEfi := "/boot/efi/EFI/ubuntu/" + testBootEntry.Filename
		err := afero.WriteFile(mockEnv.memFs, targetEfi, []byte("foo"), 0644)
		if err != nil {
			return nil, fmt.Errorf("Unable to write %s", targetEfi)
		}
	}

	// To be able to use the correct NewFileDevicePath
	var mockInterface MockEFIVariables

	// Generate bm.entries
	for _, bootNum := range entryBootNums {
		testBootEntry := entryMap[bootNum]
		targetEfi := "/boot/efi/EFI/ubuntu/" + testBootEntry.Filename
		if &testBootEntry == nil {
			testBootEntry = GenerateTestBootEntry(bootNum)
			entryMap[bootNum] = testBootEntry
			// Entry needs to exist in the mock filesystem
			err := afero.WriteFile(mockEnv.memFs, targetEfi, []byte("foo"), 0644)
			if err != nil {
				return nil, fmt.Errorf("Unable to write %s", targetEfi)
			}
		}

		// Create Device Filepath from filepath
		dp, err := mockInterface.NewFileDevicePath(targetEfi, efi_linux.ShortFormPathHD)
		if err != nil {
			return nil, fmt.Errorf("Unable to derive device path: %v", err)
		}
		// Create test entry variable
		testBootEntryVar, err := CreateEntryVar(&testBootEntry, bootNum, dp)
		entries[bootNum] = *testBootEntryVar
	}

	// Encode inputs for mock efivars
	bootOrderBytes := EncodeBootOrder(bootOrder)
	entriesBytes := make(map[int][]byte)
	for bootNum, bootEntryVar := range entries {
		entryBytes, err := bootEntryVar.LoadOption.Bytes()
		if err != nil {
			return nil, fmt.Errorf("Unable to encode Boot%04X: %v", bootNum, err)
		}
		entriesBytes[bootNum] = entryBytes
	}

	mockvars := GetMockEFIVars(bootOrderBytes, entriesBytes)

	// Mock Boot Manager
	attrib := defaultAttrib()
	bm := BootManager{
		efivars:        &mockvars,
		entries:        entries,
		bootOrder:      bootOrder,
		bootOrderAttrs: attrib,
	}
	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu", &bm)
	if err != nil {
		return nil, fmt.Errorf("Could not create kernel manager: %v", err)
	}
	km.bootEntries = bootEntries
	mockEnv.km = *km
	return &mockEnv, nil
}

// SetupTestFs sets up a new mock filesystem for testing and returns the
// filesystem for additional file modification.
// Also resets the global appArchitecture and appFs variables per call.
func SetupTestFs() *afero.Fs {
	appArchitecture = "x64"

	// appFS is global and needs to be reset per test to ensure
	// environment is accurate
	memFs := afero.NewMemMapFs()

	// Create kernel source dir
	memFs.Mkdir("/usr/lib/linux", 0644)
	// Create ESP & kernel target dirs
	memFs.Mkdir("/boot/efi/EFI/ubuntu", 0644)
	appFs = MapFS{memFs}
	return &memFs
}

// GetMockEFIVars provides an interface to create a MockEFIVariables from
// a specific set of entries rather than using the Get and Set functions
// after creation.
func GetMockEFIVars(bootOrder []byte, entries map[int][]byte) MockEFIVariables {
	efiStore := map[efi.VariableDescriptor]mockEFIVariable{}
	attrib := defaultAttrib()
	if bootOrder != nil {
		efiStore[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "BootOrder"}] = mockEFIVariable{bootOrder, attrib}
	}
	for key, val := range entries {
		efiStore[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: fmt.Sprintf("Boot%04X", key)}] = mockEFIVariable{val, attrib}
	}

	return MockEFIVariables{efiStore}
}

// GenerateTestBootEntry returns a BootEntry unique to the integer used as
// input. This BootEntry is intended to be recognized as an Ubuntu kernel
// EFI.
func GenerateTestBootEntry(i int) BootEntry {
	return BootEntry{
		Filename:    fmt.Sprintf("k%d.efi", i),
		Label:       fmt.Sprintf("Ubuntu kernel %d", i),
		Options:     fmt.Sprintf(" \\ k%d", i),
		Description: fmt.Sprintf("Ubuntu kernel entry %d", i),
	}
}
