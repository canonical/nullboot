// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package luks2

import (
	"crypto/rand"
	"fmt"
	efi "github.com/canonical/go-efilib"
	"github.com/snapcore/secboot"
	_ "github.com/snapcore/secboot/luks2" // This gets the LUKS2 backend initialized
)

func CreateRecoveryKey(devicePath string, recoveryName string) error {
	fmt.Printf("Creating recovery key '%v' in '%v'\n", recoveryName, devicePath)
	container, err := secboot.FindStorageContainer(efi.DefaultVarContext, devicePath)
	if err != nil {
		fmt.Printf("Cannot find storage (LUKS) container: %v\n", err)
		return err
	}
	purpose := secboot.KeyringKeyPurposeUnlock
	// prefix "" defaults to "ubuntu-fde"
	diskUnlockKey, err := secboot.GetKeyFromKernel(efi.DefaultVarContext, container, purpose, "")
	if err != nil {
		fmt.Printf("Cannot get disk unlock key from kernel: %v\n", err)
		return err
	}

	recoveryKey := secboot.RecoveryKey{} // generate random from crypto rand package
	rand.Read(recoveryKey[:])

	keyslotName := recoveryName // anything, stored in the token
	err = secboot.AddLUKS2ContainerRecoveryKey(devicePath, keyslotName, diskUnlockKey, recoveryKey)
	// soon to be replaced. (package secboot.luks2)
	if err != nil {
		fmt.Printf("Cannot add recovery key to LUKS container: %v\n", err)
		return err
	}

	fmt.Printf("%s\n", recoveryKey.String())

	return nil
}

func ListRecoveryKeys(devicePath string) error {
	fmt.Printf("Listing recovery keys in '%v'\n", devicePath)
	recovery_names, err := secboot.ListLUKS2ContainerRecoveryKeyNames(devicePath)
	if err != nil {
		fmt.Printf("Cannot list recovery keys: %v\n", err)
		return err
	}

	for _, name := range recovery_names {
		fmt.Printf("%v\n", name)
	}
	return nil
}

func DeleteRecoveryKey(devicePath string, recoveryName string) error {
	fmt.Printf("Deleting recovery key '%v' in '%v'\n", recoveryName, devicePath)
	err := secboot.DeleteLUKS2ContainerKey(devicePath, recoveryName)
	if err != nil {
		fmt.Printf("Cannot delete recovery key: %v\n", err)
		return err
	}
	return nil
}
