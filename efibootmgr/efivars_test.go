// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

// This file does not contain actual tests, but contains mock implementations of EFIVariables

package efibootmgr

import (
	"errors"

	"github.com/canonical/go-efilib"

	"github.com/canonical/nullboot/efivars"
)

type NoEFIVariables struct{}

func (NoEFIVariables) ListVariables() ([]efi.VariableDescriptor, error) {
	return nil, efi.ErrVarsUnavailable
}

func (NoEFIVariables) GetVariable(guid efi.GUID, name string) ([]byte, efi.VariableAttributes, error) {
	return nil, 0, efi.ErrVarsUnavailable
}

func (NoEFIVariables) SetVariable(guid efi.GUID, name string, data []byte, attrs efi.VariableAttributes) error {
	return efi.ErrVarsUnavailable
}

func (NoEFIVariables) NewDevicePath(filepath string, options uint32) (efivars.DevicePath, error) {
	return nil, errors.New("Cannot access")
}

type mockEFIVariable struct {
	data  []byte
	attrs efi.VariableAttributes
}

type MockEFIVariables struct {
	store map[efi.VariableDescriptor]mockEFIVariable
}

func (m MockEFIVariables) ListVariables() (out []efi.VariableDescriptor, err error) {
	for k := range m.store {
		out = append(out, k)
	}
	return out, nil
}

func (m MockEFIVariables) GetVariable(guid efi.GUID, name string) (data []byte, attrs efi.VariableAttributes, err error) {
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

func (m MockEFIVariables) NewDevicePath(filepath string, options uint32) (efivars.DevicePath, error) {
	file, err := appFs.Open(filepath)
	if err != nil {
		return nil, err
	}
	file.Close()
	return efivars.LoadOption{Data: UsbrBootCdrom}.Path(), nil
}

var UsbrBootCdrom = []byte{9, 0, 0, 0, 28, 0, 85, 0, 83, 0, 66, 0, 82, 0, 32, 0, 66, 0, 79, 0, 79, 0, 84, 0, 32, 0, 67, 0, 68, 0, 82, 0, 79, 0, 77, 0, 0, 0, 2, 1, 12, 0, 208, 65, 3, 10, 0, 0, 0, 0, 1, 1, 6, 0, 0, 20, 3, 5, 6, 0, 11, 1, 127, 255, 4, 0}
