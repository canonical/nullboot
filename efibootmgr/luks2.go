// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"crypto/rand"
	"fmt"
	"log"
	"github.com/snapcore/secboot"
	_ "github.com/snapcore/secboot/luks2" // This gets the LUKS2 backend initialized
	efi "github.com/canonical/go-efilib"
)

func SetupRecoveryKey(devicePath string, recoveryName string) error {
	container, err := secboot.FindStorageContainer(efi.DefaultVarContext, devicePath)
	if err != nil {
		log.Printf("Cannot FindStorageContainer")
		return err
	}
	purpose := secboot.KeyringKeyPurposeUnlock
	// prefix "" defaults to "ubuntu-fde"
	diskUnlockKey, err := secboot.GetKeyFromKernel(efi.DefaultVarContext, container, purpose, "")
	if err != nil {
		log.Printf("Cannot get disk unlock key from kernel")
		return err
	}

	recoveryKey := secboot.RecoveryKey{} // generate random from crypto rand package
	rand.Read(recoveryKey[:])

	keyslotName := recoveryName // anything, stored in the token
	err = secboot.AddLUKS2ContainerRecoveryKey(devicePath, keyslotName, diskUnlockKey, recoveryKey)
	// soon to be replaced. (package secboot.luks2)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", recoveryKey.String())

	return nil
}

func ListRecoveryPassphrases(devicePath string) error {
	recovery_names, err := secboot.ListLUKS2ContainerRecoveryKeyNames(devicePath)
	if err != nil {
		log.Printf("Cannot ListLUKS2ContainerRecoveryKeyNames: %v", err)
		return err
	}

	for _, name := range recovery_names {
		fmt.Printf("%v\n", name)
	}
	return nil
}

func DeleteRecoveryPassphrase(devicePath string, recoveryName string) error {
	err := secboot.DeleteLUKS2ContainerKey(devicePath, recoveryName)
	if err != nil {
		log.Printf("Cannot DeleteLUKS2ContainerKey: %v", err)
		return err
	}
	return nil
}
