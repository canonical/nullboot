// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"github.com/canonical/bootmgrless/efivars"
	"os"
)

// EFIVariables abstracts away the host-specific bits of the efivars module
type EFIVariables interface {
	GetVariableNames(filterGUID efivars.GUID) []string
	GetVariable(guid efivars.GUID, name string) (data []byte, attrs uint32)
	VariablesSupported() bool
	SetVariable(guid efivars.GUID, name string, data []byte, attrs uint32, mode os.FileMode) error
	NewDevicePath(filepath string, options uint32) (efivars.DevicePath, error)
}

// RealEFIVariables provides the real implementation of efivars
type RealEFIVariables struct{}

// GetVariableNames proxy
func (RealEFIVariables) GetVariableNames(filterGUID efivars.GUID) []string {
	return efivars.GetVariableNames(filterGUID)
}

// GetVariable proxy
func (RealEFIVariables) GetVariable(guid efivars.GUID, name string) (data []byte, attrs uint32) {
	return efivars.GetVariable(guid, name)
}

// VariablesSupported proxy
func (RealEFIVariables) VariablesSupported() bool { return efivars.VariablesSupported() }

// SetVariable proxy
func (RealEFIVariables) SetVariable(guid efivars.GUID, name string, data []byte, attrs uint32, mode os.FileMode) error {
	return efivars.SetVariable(guid, name, data, attrs, mode)
}

// NewDevicePath proxy
func (RealEFIVariables) NewDevicePath(filepath string, options uint32) (efivars.DevicePath, error) {
	return efivars.NewDevicePath(filepath, options)
}
