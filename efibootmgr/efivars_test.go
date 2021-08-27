// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

// This file does not contain actual tests, but contains mock implementations of EFIVariables

package efibootmgr

import (
	"errors"
	"fmt"
	"github.com/canonical/nullboot/efivars"
	"os"
)

type NoEFIVariables struct{}

func (NoEFIVariables) GetVariableNames(filterGUID efivars.GUID) []string { return nil }
func (NoEFIVariables) GetVariable(guid efivars.GUID, name string) (data []byte, attrs uint32) {
	return nil, 0
}
func (NoEFIVariables) VariablesSupported() bool { return false }
func (NoEFIVariables) SetVariable(guid efivars.GUID, name string, data []byte, attrs uint32, mode os.FileMode) error {
	return errors.New("Not implemented")
}
func (NoEFIVariables) DelVariable(guid efivars.GUID, name string) error {
	return errors.New("Not implemented")
}
func (NoEFIVariables) NewDevicePath(filepath string, options uint32) (efivars.DevicePath, error) {
	return nil, errors.New("Cannot access")
}

type mockEFIVariable struct {
	data  []byte
	attrs uint32
}
type MockEFIVariables struct {
	store map[efivars.GUID]map[string]mockEFIVariable
}

func (m MockEFIVariables) GetVariableNames(filterGUID efivars.GUID) []string {
	var out []string
	for name := range m.store[filterGUID] {
		out = append(out, name)
	}
	return out
}
func (m MockEFIVariables) GetVariable(guid efivars.GUID, name string) (data []byte, attrs uint32) {
	out := m.store[guid][name]
	return out.data, out.attrs
}
func (m MockEFIVariables) VariablesSupported() bool { return true }
func (m *MockEFIVariables) SetVariable(guid efivars.GUID, name string, data []byte, attrs uint32, mode os.FileMode) error {
	if m.store == nil {
		m.store = make(map[efivars.GUID]map[string]mockEFIVariable)
	}
	if _, ok := m.store[guid]; !ok {
		m.store[guid] = make(map[string]mockEFIVariable)
	}
	m.store[guid][name] = mockEFIVariable{data, attrs}
	return nil
}
func (m MockEFIVariables) DelVariable(guid efivars.GUID, name string) error {
	if _, ok := m.store[guid][name]; !ok {
		return fmt.Errorf("Could not delete non-existing variable %s", name)
	}
	delete(m.store[guid], name)
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
