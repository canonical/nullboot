// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"sort"
	"strings"

	"github.com/knqyf263/go-deb-version"
)

const kernelPrefix = "kernel.efi-"

type Kernel struct {
	Version  version.Version
	FilePath string
}

func (k *Kernel) GetKernelName() string {
	return path.Base(k.FilePath)
}

func (k *Kernel) Equals(other Kernel) bool {
	return k.Version.Equal(other.Version) && k.GetKernelName() == other.GetKernelName()
}

func NewKernel(kernelPath string) (Kernel, error) {
	kernelName := path.Base(kernelPath)
	if versionStr, err := getKernelABI(kernelName); err == nil {
		v, err := version.NewVersion(versionStr)
		if err != nil {
			return Kernel{}, fmt.Errorf("could not parse kernel version of %s: %w", kernelName, err)
		}
		return Kernel{v, kernelPath}, nil
	}
	return Kernel{}, fmt.Errorf("unrecognized kernel naming format: %s", kernelName)
}

type KernelEntry struct {
	kernel    Kernel
	bootEntry BootEntry
}

// KernelManager manages kernels in an SP vendor directory.
//
// It will update or install shim, copy in any new kernels,
// remove old kernels, and configure boot in shim and BDS.
type KernelManager struct {
	sourceDir     string        // sourceDir is the location to copy kernels from
	targetDir     string        // targetDir is a vendor directory on the ESP
	sourceKernels []Kernel      // kernels in sourceDir
	targetKernels []Kernel      // kernels in targetDir
	kernelEntries []KernelEntry // boot entries filled by InstallKernels
	kernelOptions string        // options to pass to kernel
	bootManager   *BootManager  // The EFI boot manager
}

// NewKernelManager returns a new kernel manager managing kernels in the host system
func NewKernelManager(esp, sourceDir, vendor string, bootManager *BootManager) (*KernelManager, error) {
	var km KernelManager
	var err error

	km.sourceDir = sourceDir
	km.targetDir = path.Join(esp, "EFI", vendor)
	km.bootManager = bootManager

	if file, err := appFs.Open("/etc/kernel/cmdline"); err == nil {
		defer file.Close()
		data, err := ioutil.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("Cannot read kernel command line: %w", err)
		}

		km.kernelOptions = strings.TrimSpace(string(data))
	}

	km.sourceKernels, err = km.readKernels(km.sourceDir)
	if err != nil {
		return nil, err
	}
	km.targetKernels, err = km.readKernels(km.targetDir)
	if err != nil {
		return nil, err
	}

	return &km, nil
}

// readKernels returns a list of all kernels in the given directory
func (km *KernelManager) readKernels(dir string) ([]Kernel, error) {
	var kernels []Kernel
	entries, err := appFs.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Could not determine kernels: %w", err)
	}
	for _, e := range entries {
		if !hasKernelPrefix(e.Name()) {
			continue
		}
		kernel, err := NewKernel(path.Join(dir, e.Name()))
		if err != nil {
			return []Kernel{}, err
		}
		kernels = append(kernels, kernel)
	}
	// Sort descending
	sort.Slice(kernels, func(i, j int) bool {
		a := kernels[i].Version
		b := kernels[j].Version
		return a.GreaterThan(b)
	})
	return kernels, err
}

// getKernelABI returns the kernel ABI part of the kernel filename
func getKernelABI(kernel string) (string, error) {
	if hasKernelPrefix(kernel) {
		return kernel[len(kernelPrefix):], nil
	}
	return "", fmt.Errorf("unknown naming format of kernel %s, unable to parse ABI", kernel)
}

func hasKernelPrefix(kernel string) bool {
	return strings.HasPrefix(kernel, kernelPrefix)
}

// InstallKernels installs the kernels to the ESP and builds up the boot entries
// to commit using CommitToBootLoader()
func (km *KernelManager) InstallKernels() error {
	km.kernelEntries = nil
	for _, sk := range km.sourceKernels {
		kName := sk.GetKernelName()
		tkFilePath := path.Join(km.targetDir, kName)
		updated, err := MaybeUpdateFile(tkFilePath, sk.FilePath)
		if err != nil {
			log.Printf("Could not install kernel %s: %v", kName, err)
			continue
		}
		if updated {
			log.Printf("Installed or updated kernel %s", kName)
		}

		bootEntry := NewKernelBootEntry("Ubuntu", sk, km.kernelOptions)
		km.kernelEntries = append(km.kernelEntries, KernelEntry{sk, bootEntry})
	}

	return nil
}

// RegisterNewKernelEFIs creates EFI variables for each new kernel
// installed via InstallKernels, adding them to the BootManager and
// creating the variables on the host machine.
func (km *KernelManager) RegisterNewKernelEFIs() error {
	for _, ke := range km.kernelEntries {
		entry := ke.bootEntry
		if _, err := km.bootManager.FindOrCreateEntry(entry, km.targetDir); err != nil {
			return fmt.Errorf("unable to find or create EFI boot entry for %s: %w", entry.Label, err)
		}
	}
	return nil
}

// IsObsoleteKernel checks whether a kernel is obsolete.
func (km *KernelManager) isObsoleteKernel(k Kernel) bool {
	for _, sk := range km.sourceKernels {
		if sk.Equals(k) {
			return false
		}
	}
	return true
}

// RemoveObsoleteKernels removes old kernels in the ESP vendor directory
func (km *KernelManager) RemoveObsoleteKernels() error {
	var remaining []Kernel
	for _, tk := range km.targetKernels {
		if !km.isObsoleteKernel(tk) {
			continue
		}
		if err := appFs.Remove(tk.FilePath); err != nil {
			log.Printf("Could not remove kernel %s: %v", tk.FilePath, err)
			remaining = append(remaining, tk)
			continue
		}

		log.Printf("Removed kernel %s", tk.FilePath)
	}

	km.targetKernels = remaining

	return nil
}

// CommitToBootLoader updates the firmware BDS entries and shim's boot.csv
func (km *KernelManager) CommitToBootLoader() error {
	log.Print("Configuring shim fallback loader")
	bootEntries := []BootEntry{}
	for _, ke := range km.kernelEntries {
		bootEntries = append(bootEntries, ke.bootEntry)
	}

	// We completely own the shim fallback file, so just write it
	if err := WriteShimFallbackToFile(path.Join(km.targetDir, "BOOT"+strings.ToUpper(GetEfiArchitecture())+".CSV"), bootEntries); err != nil {
		log.Printf("Failed to configure shim fallback loader: %v", err)
	}

	if km.bootManager == nil {
		return nil
	}

	log.Print("Configuring UEFI boot device selection")

	// This will become the head of the new boot order
	var ourBootOrder []int

	// Add new entries, find existing ones and build target boot order
	for _, entry := range bootEntries {
		entryVar, err := km.bootManager.FindBootEntryVariable(entry, km.targetDir)
		if err != nil {
			return fmt.Errorf("failure to find boot entry for %s: %w", entry.Label, err)
		}
		ourBootOrder = append(ourBootOrder, entryVar.BootNumber)
	}

	// Delete any obsolete kernels
	for _, ev := range km.bootManager.entries {
		if !strings.HasPrefix(ev.LoadOption.Description, "Ubuntu ") {
			continue
		}
		isObsolete := true
		for _, num := range ourBootOrder {
			if num == ev.BootNumber {
				isObsolete = false
			}
		}
		if !isObsolete {
			continue
		}

		if err := km.bootManager.DeleteEntry(ev.BootNumber); err != nil {
			log.Printf("Could not delete Boot%04X: %v", ev.BootNumber, err)
		}
	}

	// Set the boot order
	if err := km.bootManager.PrependAndSetBootOrder(ourBootOrder); err != nil {
		return fmt.Errorf("Could not set boot order: %w", err)
	}

	return nil
}

// SetLatestKernelToBootNext sets the latest kernel to be BootNext.
//
// Returns an error if the entry does not yet exist as a BootEntryVariable
// or if there is an error setting BootNext.
func (km *KernelManager) SetLatestKernelToBootNext() error {
	latestKernelEntry, err := km.GetLatestKernelEntry()
	latestKernelBootEntry := latestKernelEntry.bootEntry
	if err != nil {
		return fmt.Errorf("unable to get latest kernel entry: %w", err)
	}
	latestKernelEntryVar, err := km.bootManager.FindBootEntryVariable(latestKernelBootEntry, km.targetDir)
	if err != nil {
		return fmt.Errorf("unable to find boot variable for %s: %w", latestKernelEntry.kernel.FilePath, err)
	}
	if err := km.bootManager.SetBootNext(latestKernelEntryVar.BootNumber); err != nil {
		return fmt.Errorf("unable to set BootNext to Boot%04X (%s): %w", latestKernelEntryVar.BootNumber, latestKernelEntry.kernel.FilePath, err)
	}

	return nil
}

func (km *KernelManager) IsCurrentBootLatest() (bool, error) {
	if len(km.kernelEntries) == 0 {
		return false, fmt.Errorf("no Ubuntu Kernel EFIs have been loaded")
	}

	latestKernelEntry, err := km.GetLatestKernelEntry()
	if err != nil {
		return false, fmt.Errorf("unable to get latest kernel entry: %w", err)
	}
	latestKernelEntryVar, err := km.bootManager.FindBootEntryVariable(latestKernelEntry.bootEntry, km.targetDir)
	if err != nil {
		return false, fmt.Errorf("unable to find latest kernel boot variable: %w", err)
	}

	// Determine if the BootEntryVariable is the BootCurrent variable
	if latestKernelEntryVar.BootNumber == km.bootManager.bootCurrent {
		return true, nil
	} else {
		return false, nil
	}
}

// Returns the KernelEntry with the largest version in km.kernelEntries
func (km *KernelManager) GetLatestKernelEntry() (KernelEntry, error) {
	numEntries := len(km.kernelEntries)
	if numEntries == 0 {
		return KernelEntry{}, fmt.Errorf("no kernels have been registered to the KernelManager")
	}
	latest := km.kernelEntries[0]
	for _, ke := range km.kernelEntries[1:] {
		curVersion := ke.kernel.Version
		if curVersion.GreaterThan(latest.kernel.Version) {
			latest = ke
		}
	}
	return latest, nil
}
