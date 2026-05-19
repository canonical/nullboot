// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package luks2

import (
	"bufio"
	"crypto/rand"
	"fmt"
	efi "github.com/canonical/go-efilib"
	"github.com/snapcore/secboot"
	_ "github.com/snapcore/secboot/luks2" // This gets the LUKS2 backend initialized
	"io"
	"os"
	"strings"
)

var (
	sbAddLUKS2ContainerRecoveryKey       = secboot.AddLUKS2ContainerRecoveryKey
	sbDeleteLUKS2ContainerKey            = secboot.DeleteLUKS2ContainerKey
	sbFindStorageContainer               = secboot.FindStorageContainer
	sbGetKeyFromKernel                   = secboot.GetKeyFromKernel
	sbListLUKS2ContainerRecoveryKeyNames = secboot.ListLUKS2ContainerRecoveryKeyNames
	sbParseRecoveryKey                   = secboot.ParseRecoveryKey
	sbTestLUKS2ContainerKey              = secboot.TestLUKS2ContainerKey

	stdin  io.Reader = os.Stdin
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

// CreateRecoveryKey creates a new recovery key for a device and prints
// it to stdout.
func CreateRecoveryKey(devicePath string, recoveryName string) error {
	fmt.Fprintf(stderr, "Creating recovery key '%v' in '%v'\n", recoveryName, devicePath)
	container, err := sbFindStorageContainer(efi.DefaultVarContext, devicePath)
	if err != nil {
		return fmt.Errorf("Cannot find storage (LUKS) container: %w", err)
	}
	purpose := secboot.KeyringKeyPurposeUnlock
	// prefix "" defaults to "ubuntu-fde"
	diskUnlockKey, err := sbGetKeyFromKernel(efi.DefaultVarContext, container, purpose, "")
	if err != nil {
		return fmt.Errorf("Cannot get disk unlock key from kernel: %w", err)
	}

	recoveryKey := secboot.RecoveryKey{} // generate random from crypto rand package
	rand.Read(recoveryKey[:])

	keyslotName := recoveryName // anything, stored in the token
	err = sbAddLUKS2ContainerRecoveryKey(devicePath, keyslotName, diskUnlockKey, recoveryKey)
	if err != nil {
		return fmt.Errorf("Cannot add recovery key to LUKS container: %w", err)
	}

	fmt.Fprintf(stdout, "%s\n", recoveryKey.String())

	return nil
}

// VerifyRecoveryKey reads a recovery from stdin and verifies if it can unlock a device.
func VerifyRecoveryKey(devicePath string) error {
	reader := bufio.NewReader(stdin)
	fmt.Print("Enter recovery key: ")
	recoveryKeyString, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	recoveryKeyString = strings.TrimSpace(recoveryKeyString)

	key, err := sbParseRecoveryKey(recoveryKeyString)
	if err != nil {
		return err
	}

	ok := sbTestLUKS2ContainerKey(devicePath, key[:])
	if !ok {
		fmt.Fprintf(stdout, "fail\n")
		return fmt.Errorf("This recovery key cannot unlock the device")
	}

	fmt.Fprintf(stdout, "success\n")
	return nil
}

// ListRecoveryKeys lists recovery keys of a device.
func ListRecoveryKeys(devicePath string) error {
	fmt.Fprintf(stderr, "Listing recovery keys in '%v'\n", devicePath)
	recovery_names, err := sbListLUKS2ContainerRecoveryKeyNames(devicePath)
	if err != nil {
		return fmt.Errorf("Cannot list recovery keys: %w", err)
	}

	for _, name := range recovery_names {
		fmt.Fprintf(stdout, "%v\n", name)
	}
	return nil
}

// DeleteRecoveryKey deletes the given recovery key from a device.
func DeleteRecoveryKey(devicePath string, recoveryKeyName string) error {
	fmt.Fprintf(stdout, "Deleting recovery key '%v' in '%v'\n", recoveryKeyName, devicePath)
	err := sbDeleteLUKS2ContainerKey(devicePath, recoveryKeyName)
	if err != nil {
		return fmt.Errorf("Cannot delete recovery key: %w", err)
	}
	return nil
}
