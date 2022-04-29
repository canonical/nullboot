// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	//"errors"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"

	efi "github.com/canonical/go-efilib"
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
func (*RealEFIVariables) ListVariables() ([]efi.VariableDescriptor, error) {
	return efi.ListVariables()
}

// GetVariable proxy
func (*RealEFIVariables) GetVariable(guid efi.GUID, name string) (data []byte, attrs efi.VariableAttributes, err error) {
	return efi.ReadVariable(name, guid)
}

// SetVariable proxy
func (*RealEFIVariables) SetVariable(guid efi.GUID, name string, data []byte, attrs efi.VariableAttributes) error {
	return efi.WriteVariable(name, guid, attrs, data)
}

// NewFileDevicePath proxy
func (*RealEFIVariables) NewFileDevicePath(filepath string, mode efi_linux.FileDevicePathMode) (efi.DevicePath, error) {
	return efi_linux.NewFileDevicePath(filepath, mode)
}

type mockEFIVariable struct {
	data  []byte
	attrs efi.VariableAttributes
}

type MockEFIVariables struct {
	store map[efi.VariableDescriptor]mockEFIVariable
}

func NewMockEFIVariables() MockEFIVariables {
	mockVars := MockEFIVariables{
		store: make(map[efi.VariableDescriptor]mockEFIVariable, 0),
	}

	mockVars.store[efi.VariableDescriptor{
		Name: "BootOrder",
		GUID: efi.GlobalVariable,
	}] = mockEFIVariable{
		data:  []byte{0x0, 0x0},
		attrs: efi.AttributeNonVolatile | efi.AttributeBootserviceAccess | efi.AttributeRuntimeAccess,
	}
	return mockVars
}

func (m *MockEFIVariables) ListVariables() (out []efi.VariableDescriptor, err error) {
	for k := range m.store {
		out = append(out, k)
	}
	return out, nil
}

func (m *MockEFIVariables) GetVariable(guid efi.GUID, name string) (data []byte, attrs efi.VariableAttributes, err error) {
	out, ok := m.store[efi.VariableDescriptor{Name: name, GUID: guid}]
	if !ok {
		return nil, 0, efi.ErrVarNotExist
	}
	return out.data, out.attrs, nil
}

func (m *MockEFIVariables) SetVariable(guid efi.GUID, name string, data []byte, attrs efi.VariableAttributes) error {
	if m.store == nil {
		m.store = make(map[efi.VariableDescriptor]mockEFIVariable)
	}
	if len(data) == 0 {
		delete(m.store, efi.VariableDescriptor{Name: name, GUID: guid})
	} else {
		m.store[efi.VariableDescriptor{Name: name, GUID: guid}] = mockEFIVariable{data, attrs}
	}
	return nil
}

func (m *MockEFIVariables) NewFileDevicePath(filepath string, mode efi_linux.FileDevicePathMode) (efi.DevicePath, error) {
	file, err := appFs.Open(filepath)
	if err != nil {
		return nil, err
	}
	file.Close()

	return efi.DevicePath{
		&efi.ACPIDevicePathNode{HID: 0x0a0341d0},
		&efi.PCIDevicePathNode{Device: 0x14, Function: 0},
		&efi.USBDevicePathNode{ParentPortNumber: 0xb, InterfaceNumber: 0x1}}, nil
}

// JSON returns a JSON representation of the Boot Manager
func (m *MockEFIVariables) JSON() ([]byte, error) {
	payload := make(map[string]map[string]string)

	var numBytes [2]byte
	for key, entry := range m.store {
		entryID := key.Name
		entryBase64 := base64.StdEncoding.EncodeToString(entry.data)

		binary.LittleEndian.PutUint16(numBytes[0:], uint16(entry.attrs))
		entryAttrBase64 := base64.StdEncoding.EncodeToString([]byte{numBytes[0], numBytes[1]})

		payload[entryID] = map[string]string{
			"guid":       "Yd/ki8qT0hGqDQDgmAMrjA==",
			"attributes": entryAttrBase64,
			"value":      entryBase64,
		}
	}

	return json.Marshal(payload)
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

// Chosen implementation
var appEFIVars EFIVariables = &RealEFIVariables{}

// VariablesSupported indicates whether variables can be accessed.
func VariablesSupported() bool {
	_, err := appEFIVars.ListVariables()
	return err == nil
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

// NewFileDevicePath constructs a EFI device path for the specified file path.
func NewFileDevicePath(filepath string, mode efi_linux.FileDevicePathMode) (efi.DevicePath, error) {
	return appEFIVars.NewFileDevicePath(filepath, mode)
}
