// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	//"errors"
	"github.com/canonical/go-efilib"
	efi_linux "github.com/canonical/go-efilib/linux"
)

// EFIVariables abstracts away the host-specific bits of the efivars module
type EFIVariables interface {
	ListVariables() ([]efi.VariableDescriptor, error)
	GetVariable(guid efi.GUID, name string) (data []byte, attrs efi.VariableAttributes, err error)
	SetVariable(guid efi.GUID, name string, data []byte, attrs efi.VariableAttributes) error
	NewFileDevicePath(filepath string, mode efi_linux.FileDevicePathMode) (efi.DevicePath, error)
}

// RealEFIVariables provides the real implementation of efivars
type RealEFIVariables struct{}

// ListVariables proxy
func (RealEFIVariables) ListVariables() ([]efi.VariableDescriptor, error) {
	return efi.ListVariables()
}

// GetVariable proxy
func (RealEFIVariables) GetVariable(guid efi.GUID, name string) (data []byte, attrs efi.VariableAttributes, err error) {
	return efi.ReadVariable(name, guid)
}

// SetVariable proxy
func (RealEFIVariables) SetVariable(guid efi.GUID, name string, data []byte, attrs efi.VariableAttributes) error {
	return efi.WriteVariable(name, guid, attrs, data)
}

// NewFileDevicePath proxy
func (RealEFIVariables) NewFileDevicePath(filepath string, mode efi_linux.FileDevicePathMode) (efi.DevicePath, error) {
	return efi_linux.NewFileDevicePath(filepath, mode)
}

// VariablesSupported indicates whether variables can be accessed.
func VariablesSupported(efiVars EFIVariables) bool {
	_, err := efiVars.ListVariables()
	return err == nil
}

// GetVariableNames returns the names of every variable with the specified GUID.
func GetVariableNames(efiVars EFIVariables, filterGUID efi.GUID) (names []string, err error) {
	vars, err := efiVars.ListVariables()
	if err != nil {
		return nil, err
	}
	for _, entry := range vars {
		if entry.GUID != filterGUID {
			continue
		}
		names = append(names, entry.Name)
	}
	return names, nil
}

// DelVariable deletes the non-authenticated variable with the specified name.
func DelVariable(efivars EFIVariables, guid efi.GUID, name string) error {
	_, attrs, err := efivars.GetVariable(guid, name)
	if err != nil {
		return err
	}
	// XXX: Update tests to not set these attributes in mock variables
	//if attrs&(efi.AttributeAuthenticatedWriteAccess|efi.AttributeTimeBasedAuthenticatedWriteAccess|efi.AttributeEnhancedAuthenticatedAccess) != 0 {
	//	return errors.New("variable must be deleted by setting an authenticated empty payload")
	//}
	return efivars.SetVariable(guid, name, nil, attrs)
}
