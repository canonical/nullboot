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

// KernelManager manages kernels in an SP vendor directory.
//
// It will update or install shim, copy in any new kernels,
// remove old kernels, and configure boot in shim and BDS.
type KernelManager struct {
	sourceDir     string       // sourceDir is the location to copy kernels from
	targetDir     string       // targetDir is a vendor directory on the ESP
	sourceKernels []string     // kernels in sourceDir
	targetKernels []string     // kernels in targetDir
	bootEntries   []BootEntry  // boot entries filled by InstallKernels
	kernelOptions string       // options to pass to kernel
	bootManager   *BootManager // The EFI boot manager
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

// readKernels returns a list of all kernels in the
func (km *KernelManager) readKernels(dir string) ([]string, error) {
	var kernels []string
	entries, err := appFs.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Could not determine kernels: %w", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "kernel.efi-") {
			kernels = append(kernels, e.Name())
		}
	}
	// Sort descending
	sort.Slice(kernels, func(i, j int) bool {
		a, e := version.NewVersion(kernels[i][len("kernel.efi-"):])
		if e != nil {
			err = fmt.Errorf("Could not parse kernel version of %s: %w", kernels[i], e)
			return false
		}
		b, e := version.NewVersion(kernels[j][len("kernel.efi-"):])
		if e != nil {
			err = fmt.Errorf("Could not parse kernel version of %s: %w", kernels[j], e)
			return false
		}
		return a.GreaterThan(b)
	})
	return kernels, err
}

// getKernelABI returns the kernel ABI part of the kernel filename
func getKernelABI(kernel string) string {
	return kernel[len("kernel.efi-"):]
}

// InstallKernels installs the kernels to the ESP and builds up the boot entries
// to commit using CommitToBootLoader()
func (km *KernelManager) InstallKernels() error {
	km.bootEntries = nil
	for _, sk := range km.sourceKernels {
		updated, err := MaybeUpdateFile(path.Join(km.targetDir, sk),
			path.Join(km.sourceDir, sk))
		if err != nil {
			log.Printf("Could not install kernel %s: %v", sk, err)
			continue
		}
		if updated {
			log.Printf("Installed or updated kernel %s", sk)
		}
		km.bootEntries = append(
			km.bootEntries,
			NewKernelBootEntry("Ubuntu", sk, km.kernelOptions),
		)
	}

	return nil
}

// RegisterNewKernelEFIs creates EFI variables for each new kernel
// installed via InstallKernels, adding them to the BootManager and
// creating the variables on the host machine.
func (km *KernelManager) RegisterNewKernelEFIs() error {
	for _, entry := range km.bootEntries {
		if _, err := km.bootManager.FindOrCreateEntry(entry, km.targetDir); err != nil {
			return fmt.Errorf("unable to find or create EFI boot entry for %s: %w", entry.Label, err)
		}
	}
	return nil
}

// IsObsoleteKernel checks whether a kernel is obsolete.
func (km *KernelManager) isObsoleteKernel(k string) bool {
	for _, sk := range km.sourceKernels {
		if sk == k {
			return false
		}
	}
	return true
}

// RemoveObsoleteKernels removes old kernels in the ESP vendor directory
func (km *KernelManager) RemoveObsoleteKernels() error {
	var remaining []string
	for _, tk := range km.targetKernels {
		if !km.isObsoleteKernel(tk) {
			continue
		}
		if err := appFs.Remove(path.Join(km.targetDir, tk)); err != nil {
			log.Printf("Could not remove kernel %s: %v", tk, err)
			remaining = append(remaining, tk)
			continue
		}

		log.Printf("Removed kernel %s", tk)
	}

	km.targetKernels = remaining

	return nil
}

// CommitToBootLoader updates the firmware BDS entries and shim's boot.csv
func (km *KernelManager) CommitToBootLoader() error {
	log.Print("Configuring shim fallback loader")

	// We completely own the shim fallback file, so just write it
	if err := WriteShimFallbackToFile(path.Join(km.targetDir, "BOOT"+strings.ToUpper(GetEfiArchitecture())+".CSV"), km.bootEntries); err != nil {
		log.Printf("Failed to configure shim fallback loader: %v", err)
	}

	if km.bootManager == nil {
		return nil
	}

	log.Print("Configuring UEFI boot device selection")

	// This will become the head of the new boot order
	var ourBootOrder []int

	// Add new entries, find existing ones and build target boot order
	for _, entry := range km.bootEntries {
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
	latestKernel, err := km.GetLatestKernelEntry()
	if err != nil {
		return fmt.Errorf("unable to get latest kernel entry: %w", err)
	}
	latestKernelEntryVar, err := km.bootManager.FindBootEntryVariable(latestKernel, km.targetDir)
	if err != nil {
		return fmt.Errorf("unable to find boot variable for %s, %v: %w", latestKernel.Label, latestKernel.Options, err)
	}
	if err := km.bootManager.SetBootNext(latestKernelEntryVar.BootNumber); err != nil {
		return fmt.Errorf("unable to set BootNext to Boot%04X (%s): %w", latestKernelEntryVar.BootNumber, latestKernel.Label, err)
	}

	return nil
}

func (km *KernelManager) IsCurrentBootLatest() (bool, error) {
	if len(km.bootEntries) == 0 {
		return false, fmt.Errorf("no Ubuntu Kernel EFIs have been loaded")
	}

	latestKernelEntry, err := km.GetLatestKernelEntry()
	if err != nil {
		return false, fmt.Errorf("unable to get latest kernel entry: %w", err)
	}
	latestKernelEntryVar, err := km.bootManager.FindBootEntryVariable(latestKernelEntry, km.targetDir)
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

func (km *KernelManager) GetLatestKernelEntry() (BootEntry, error) {
	// NOTE: km.bootEntries[0] is expected to be latest kernel due to the
	// readKernels method that orders kernels by version before they are
	// installed and populated into bootEntries via InstallKernels
	if len(km.bootEntries) > 0 {
		return km.bootEntries[0], nil
	}
	return BootEntry{}, fmt.Errorf("no kernels have been registered to the KernelManager")
}
