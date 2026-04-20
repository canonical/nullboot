// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"crypto/rand"
	"fmt"
	"log"
	"encoding/hex"
	"github.com/snapcore/secboot"
	efi "github.com/canonical/go-efilib"
)

func SetupRecoveryKey(devicePath string) error {
	fmt.Printf("SetupRecoveryKey: devicePath=%s\n", devicePath)

	unlockKeyNames, err := secboot.ListLUKS2ContainerUnlockKeyNames(devicePath)
	if err != nil {
		log.Printf("unlockKeyNames: %v items", len(unlockKeyNames))
		for v := range unlockKeyNames {
			log.Printf("unlock key name: %s", v)
		}
	} else  {
		log.Printf("error in ListLUKS2ContainerUnlockKeyNames")
	}

	keyslotName := "a recovery key" // anything, stored in the token
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
	keyHex := make([]byte, hex.EncodedLen(len(diskUnlockKey))) // TODO remove
	hex.Encode(keyHex, diskUnlockKey[:]) // TODO remove
	log.Printf("key: %s\n", keyHex)      // TODO remove

    recoveryKey := secboot.RecoveryKey{} // generate random from crypto rand package
	rand.Read(recoveryKey[:])
	err = secboot.AddLUKS2ContainerRecoveryKey(devicePath, keyslotName, diskUnlockKey, recoveryKey)
    // soon to be replaced. (package secboot.luks2)
    if err != nil {
		log.Printf("Cannot AddLUKS2ContainerRecoveryKey")
		return err
    }

	fmt.Printf(recoveryKey.String())

	return nil
}
