// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package luks2

import (
	"bytes"
	"context"
	"fmt"
	"github.com/snapcore/secboot"
	"gopkg.in/check.v1"
	"os"
	"reflect"
	"testing"
)

func Test(t *testing.T) { check.TestingT(t) }

type luks2Suite struct {
	restoreFindStorageContainer               func(context.Context, string) (secboot.StorageContainer, error)
	restoreGetKeyFromKernel                   func(context.Context, secboot.StorageContainer, secboot.KeyringKeyPurpose, string) ([]byte, error)
	restoreAddLUKS2ContainerRecoveryKey       func(string, string, secboot.DiskUnlockKey, secboot.RecoveryKey) error
	restoreListLUKS2ContainerRecoveryKeyNames func(string) ([]string, error)
	restoreDeleteLUKS2ContainerKey            func(string, string) error
	restoreParseRecoveryKey                   func(string) (secboot.RecoveryKey, error)
	restoreTestLUKS2ContainerKey              func(string, []byte) bool

	mockStdin  bytes.Buffer
	mockStdout bytes.Buffer
	mockStderr bytes.Buffer
}

var containers map[string]*mockStorageContainer

var _ = check.Suite(&luks2Suite{})

type recoveryKeyStruct struct {
	keyslotName string // name of the associated keyslot
	parentKey   []byte // the unlock key that was used to create the recovery key
	recoveryKey secboot.RecoveryKey
}

type mockStorageContainer struct {
	path         string
	recoveryKeys []recoveryKeyStruct
}

func (sc mockStorageContainer) Path() string {
	return sc.path
}

func (sc mockStorageContainer) BackendName() string {
	return "backend-1"
}

func (sc mockStorageContainer) CredentialName() string {
	return "credential-name-1"
}

func (sc mockStorageContainer) Activate(ctx context.Context, ks secboot.Keyslot, key []byte, cfg secboot.ActivateConfigGetter) error {
	return nil
}

func (sc mockStorageContainer) Deactivate(ctx context.Context) error {
	return nil
}

func (sc mockStorageContainer) OpenRead(ctx context.Context) (secboot.StorageContainerReader, error) {
	return nil, nil
}

func (s *luks2Suite) SetUpTest(c *check.C) {
	s.restoreFindStorageContainer = sbFindStorageContainer
	sbFindStorageContainer = mockFindStorageContainer

	s.restoreGetKeyFromKernel = sbGetKeyFromKernel
	sbGetKeyFromKernel = mockGetKeyFromKernel

	s.restoreAddLUKS2ContainerRecoveryKey = sbAddLUKS2ContainerRecoveryKey
	sbAddLUKS2ContainerRecoveryKey = mockAddLUKS2ContainerRecoveryKey

	s.restoreListLUKS2ContainerRecoveryKeyNames = sbListLUKS2ContainerRecoveryKeyNames
	sbListLUKS2ContainerRecoveryKeyNames = mockListLUKS2ContainerRecoveryKeyNames

	s.restoreDeleteLUKS2ContainerKey = sbDeleteLUKS2ContainerKey
	sbDeleteLUKS2ContainerKey = mockDeleteLUKS2ContainerKey

	s.restoreParseRecoveryKey = sbParseRecoveryKey
	sbParseRecoveryKey = mockParseRecoveryKey

	s.restoreTestLUKS2ContainerKey = sbTestLUKS2ContainerKey
	sbTestLUKS2ContainerKey = mockTestLUKS2ContainerKey

	stderr = &s.mockStderr
	stdout = &s.mockStdout
	stdin = &s.mockStdin
}

func (s *luks2Suite) TearDownTest(c *check.C) {
	sbFindStorageContainer = s.restoreFindStorageContainer
	sbGetKeyFromKernel = s.restoreGetKeyFromKernel
	sbAddLUKS2ContainerRecoveryKey = s.restoreAddLUKS2ContainerRecoveryKey
	sbListLUKS2ContainerRecoveryKeyNames = s.restoreListLUKS2ContainerRecoveryKeyNames
	stdout = os.Stdout
	stderr = os.Stderr
	stdin = os.Stdin
}

func mockFindStorageContainer(ctx context.Context, path string) (secboot.StorageContainer, error) {
	container, ok := containers[path]
	if ok {
		return container, nil
	}
	return nil, fmt.Errorf("no such container")
}

var masterKey = []byte("12345678")
var badRecoveryKey = []byte("9999999999999999")

func mockGetKeyFromKernel(ctx context.Context, storage secboot.StorageContainer, purpose secboot.KeyringKeyPurpose, prefix string) ([]byte, error) {
	if secboot.KeyringKeyPurposeUnlock != purpose {
		return nil, fmt.Errorf("invalid purpose")
	}
	if masterKey == nil {
		return nil, fmt.Errorf("no master key")
	}
	return masterKey, nil
}

func mockAddLUKS2ContainerRecoveryKey(devicePath, keyslotName string, existingKey secboot.DiskUnlockKey, recoveryKey secboot.RecoveryKey) error {
	if devicePath == "/dev/raise-error-in-AddLUKS2ContainerRecoveryKey" {
		return fmt.Errorf("error triggered in AddLUKS2ContainerRecoveryKey")
	}

	container, ok := containers[devicePath]
	if !ok {
		return fmt.Errorf("no such container")
	}
	container.recoveryKeys = append(container.recoveryKeys, recoveryKeyStruct{
		keyslotName: keyslotName,
		parentKey:   existingKey,
		recoveryKey: recoveryKey,
	})
	return nil
}

func mockListLUKS2ContainerRecoveryKeyNames(devicePath string) ([]string, error) {
	container, ok := containers[devicePath]
	if !ok {
		return []string{}, fmt.Errorf("no such container")
	}
	names := []string{}
	for _, recoveryKey := range container.recoveryKeys {
		names = append(names, recoveryKey.keyslotName)
	}

	return names, nil
}

func mockDeleteLUKS2ContainerKey(devicePath, keyslotName string) error {
	container, ok := containers[devicePath]
	if !ok {
		return fmt.Errorf("no such container")
	}
	resultingKeys := []recoveryKeyStruct{}
	for _, recoveryKey := range container.recoveryKeys {
		if recoveryKey.keyslotName != keyslotName {
			// do not delete this one, add it in the result
			resultingKeys = append(resultingKeys, recoveryKey)
		} // else ignore it
	}
	container.recoveryKeys = resultingKeys
	return nil
}

func mockParseRecoveryKey(recoveryKeyString string) (secboot.RecoveryKey, error) {
	if len(recoveryKeyString) != len(secboot.RecoveryKey{}) {
		return secboot.RecoveryKey{}, fmt.Errorf("parse error")
	}
	recoveryKeyBytes := secboot.RecoveryKey{}
	for index, c := range recoveryKeyString {
		recoveryKeyBytes[index] = byte(c)
	}
	return recoveryKeyBytes, nil
}

func mockTestLUKS2ContainerKey(devicePath string, key []byte) bool {
	_, ok := containers[devicePath]
	if !ok {
		return false
	}
	if reflect.DeepEqual(key, badRecoveryKey) {
		return false
	}
	return true
}

func (s *luks2Suite) TestCreateRecoveryKey(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/sda1"] = &mockStorageContainer{}

	err := CreateRecoveryKey("/dev/sda1", "recovery-key-1")

	c.Assert(err, check.IsNil)
	container, _ := containers["/dev/sda1"]
	c.Assert(container, check.NotNil)
	c.Check(len(container.recoveryKeys), check.Equals, 1)
	c.Check(container.recoveryKeys[0].keyslotName, check.Equals, "recovery-key-1")
	c.Check(container.recoveryKeys[0].parentKey, check.DeepEquals, masterKey)
}

func (s *luks2Suite) TestCreateRecoveryKeyErrStorageContainer(c *check.C) {
	containers = make(map[string]*mockStorageContainer)

	err := CreateRecoveryKey("/dev/sda1", "recovery-key-1")

	c.Check(err, check.NotNil)
	c.Check(err, check.ErrorMatches, "Cannot find storage \\(LUKS\\) container: .*")
}

func (s *luks2Suite) TestCreateRecoveryKeyErrKernelKeyring(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/sda1"] = &mockStorageContainer{}
	restore := masterKey
	defer func() { masterKey = restore }()
	masterKey = nil

	err := CreateRecoveryKey("/dev/sda1", "recovery-key-1")

	c.Check(err, check.NotNil)
	c.Check(err, check.ErrorMatches, "Cannot get disk unlock key from kernel: .*")
}

func (s *luks2Suite) TestCreateRecoveryKeyErrAdd(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/raise-error-in-AddLUKS2ContainerRecoveryKey"] = &mockStorageContainer{}

	err := CreateRecoveryKey("/dev/raise-error-in-AddLUKS2ContainerRecoveryKey", "recovery-key-1")

	c.Check(err, check.NotNil)
	c.Check(err, check.ErrorMatches, "Cannot add recovery key to LUKS container: .*")
}

func (s *luks2Suite) TestVerifyRecoveryKey(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/sda1"] = &mockStorageContainer{}
	s.mockStdin.Reset()
	s.mockStdin.WriteString("0123456789abcdef\n")

	err := VerifyRecoveryKey("/dev/sda1")

	c.Check(err, check.IsNil)
}

func (s *luks2Suite) TestVerifyRecoveryKeyErrParse(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/sda1"] = &mockStorageContainer{}
	s.mockStdin.Reset()
	s.mockStdin.WriteString("hello")

	err := VerifyRecoveryKey("/dev/sda1")

	c.Assert(err, check.NotNil)
	c.Check(err, check.ErrorMatches, "parse error")
}

func (s *luks2Suite) TestVerifyRecoveryKeyErrBadKey(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/sda1"] = &mockStorageContainer{}
	s.mockStdin.Reset()
	s.mockStdin.WriteString(string(badRecoveryKey))

	err := VerifyRecoveryKey("/dev/sda1")

	c.Assert(err, check.NotNil)
	c.Check(err, check.ErrorMatches, "This recovery key cannot unlock the device")
}

func (s *luks2Suite) TestListRecoveryKeys(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/sda1"] = &mockStorageContainer{
		path: "/dev/sda1",
		recoveryKeys: []recoveryKeyStruct{
			{keyslotName: "recovery-1"},
			{keyslotName: "recovery-2"},
		},
	}
	s.mockStdout.Reset()

	err := ListRecoveryKeys("/dev/sda1")

	c.Assert(err, check.IsNil)
	c.Check(s.mockStdout.String(), check.Equals, "recovery-1\nrecovery-2\n")
}

func (s *luks2Suite) TestListRecoveryKeysErrNoSuchDevice(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/sda1"] = &mockStorageContainer{path: "/dev/sda1"}

	err := ListRecoveryKeys("/dev/sdaXXX")

	c.Assert(err, check.NotNil)
	c.Check(err, check.ErrorMatches, "Cannot list recovery keys: .*")
}

func (s *luks2Suite) TestDeleteRecoveryKey(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/sda1"] = &mockStorageContainer{
		path: "/dev/sda1",
		recoveryKeys: []recoveryKeyStruct{
			{keyslotName: "recovery-1"},
			{keyslotName: "recovery-2"},
			{keyslotName: "recovery-3"},
		},
	}

	err := DeleteRecoveryKey("/dev/sda1", "recovery-2")

	c.Assert(err, check.IsNil)
	container, _ := containers["/dev/sda1"]
	c.Assert(container, check.NotNil)

	c.Check(len(container.recoveryKeys), check.Equals, 2)
	c.Check(container.recoveryKeys[0].keyslotName, check.Equals, "recovery-1")
	c.Check(container.recoveryKeys[1].keyslotName, check.Equals, "recovery-3")
}

func (s *luks2Suite) TestDeleteRecoveryKeyErrNoSuchDevice(c *check.C) {
	containers = make(map[string]*mockStorageContainer)
	containers["/dev/sda1"] = &mockStorageContainer{path: "/dev/sda1"}

	err := DeleteRecoveryKey("/dev/sdaXXX", "recovery-2")

	c.Assert(err, check.NotNil)
	c.Check(err, check.ErrorMatches, "Cannot delete recovery key: .*")
}
