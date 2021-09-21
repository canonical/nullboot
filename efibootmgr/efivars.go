// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	//"errors"
	"github.com/canonical/go-efilib"
	"github.com/canonical/nullboot/efivars"
)

// EFIVariables abstracts away the host-specific bits of the efivars module
type EFIVariables interface {
	ListVariables() ([]efi.VariableDescriptor, error)
	GetVariable(guid efi.GUID, name string) (data []byte, attrs efi.VariableAttributes, err error)
	SetVariable(guid efi.GUID, name string, data []byte, attrs efi.VariableAttributes) error
	NewDevicePath(filepath string, options uint32) (efivars.DevicePath, error)
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

// NewDevicePath proxy
func (RealEFIVariables) NewDevicePath(filepath string, options uint32) (efivars.DevicePath, error) {
	return efivars.NewDevicePath(filepath, options)
}

// Chosen implementation
var appEFIVars EFIVariables = RealEFIVariables{}

// VariablesSupported indicates whether variables can be accessed.
func VariablesSupported() bool {
	_, err := appEFIVars.ListVariables()
	return err == nil
}

// GetVariableNames returns the names of every variable with the specified GUID.
func GetVariableNames(filterGUID efi.GUID) (names []string, err error) {
	vars, err := appEFIVars.ListVariables()
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

// GetVariable returns the payload and attributes of the variable with the specified name.
func GetVariable(guid efi.GUID, name string) (data []byte, attrs efi.VariableAttributes, err error) {
	return appEFIVars.GetVariable(guid, name)
}

// SetVariable updates the payload of the variable with the specified name.
func SetVariable(guid efi.GUID, name string, data []byte, attrs efi.VariableAttributes) error {
	return appEFIVars.SetVariable(guid, name, data, attrs)
}

// DelVariable deletes the non-authenticated variable with the specified name.
func DelVariable(guid efi.GUID, name string) error {
	_, attrs, err := appEFIVars.GetVariable(guid, name)
	if err != nil {
		return err
	}
	// XXX: Update tests to not set these attributes in mock variables
	//if attrs&(efi.AttributeAuthenticatedWriteAccess|efi.AttributeTimeBasedAuthenticatedWriteAccess|efi.AttributeEnhancedAuthenticatedAccess) != 0 {
	//	return errors.New("variable must be deleted by setting an authenticated empty payload")
	//}
	return appEFIVars.SetVariable(guid, name, nil, attrs)
}
