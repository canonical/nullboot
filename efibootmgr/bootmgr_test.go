// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/canonical/go-efilib"
	"github.com/spf13/afero"
)

func TestBootManager_mocked(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	afero.WriteFile(memFs, "path", []byte("file a"), 0644)
	afero.WriteFile(memFs, "path2", []byte("file b"), 0644)
	mockvars := MockEFIVariables{
		map[efi.VariableDescriptor]mockEFIVariable{
			{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{1, 0, 2, 0, 3, 0}, 123},
			{GUID: efi.GlobalVariable, Name: "Boot0001"}:  {UsbrBootCdromOptBytes, 42},
		},
	}

	bm, err := NewBootManagerForVariables(&mockvars)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(bm.entries) != 1 {
		t.Fatalf("Could not parse an entry")
	}

	if want := []int{1, 2, 3}; !reflect.DeepEqual(bm.bootOrder, want) {
		t.Fatalf("Expected %v, got: %v", want, bm.bootOrder)
	}

	want := BootEntryVariable{1, UsbrBootCdromOptBytes, 42, UsbrBootCdromOpt}
	if !reflect.DeepEqual(bm.entries[1], want) {
		t.Fatalf("\n"+
			"expected: %+v\n"+
			"got:      %+v", want, bm.entries[1])
	}

	// This creates entry Boot0000
	got, err := bm.FindOrCreateEntry(BootEntry{Filename: "path", Label: "desc", Options: "arg1 arg2"}, "")
	if err != nil {
		t.Fatalf("could not create next boot entry, error: %v", err)
	}
	if want := 0; got != want {
		t.Fatalf("expected to create Boot%04X, created Boot%04X", want, got)
	}

	boot0000, ok := mockvars.store[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "Boot0000"}]
	if !ok {
		t.Fatal("Variable Boot0000 does not exist")
	}

	if want := efi.AttributeNonVolatile | efi.AttributeBootserviceAccess | efi.AttributeRuntimeAccess; want != boot0000.attrs {
		t.Fatalf("Expected attributes %v, got %v", want, boot0000.attrs)
	}
	optGot, err := efi.ReadLoadOption(bytes.NewReader(boot0000.data))
	if err != nil {
		t.Fatalf("Cannot decode load option: %v", err)
	}
	descGot := optGot.Description
	if want := "desc"; want != descGot {
		t.Fatalf("Expected desc %v, got %v", want, descGot)
	}
	// This is our mock path
	pathGot := optGot.FilePath
	if want := (efi.DevicePath{efi.NewFilePathDevicePathNode("path")}); !reflect.DeepEqual(want, pathGot) {
		t.Fatalf("Expected path %v, got %v", want, pathGot)
	}

	// This creates entry Boot0002
	got, err = bm.FindOrCreateEntry(BootEntry{Filename: "path2", Label: "desc2", Options: "arg3 arg4"}, "")
	if want := 2; got != want {
		t.Fatalf("expected to create Boot%04X, created Boot%04X", want, got)
	}
	if err != nil {
		t.Fatalf("could not create next boot entry, error: %v", err)
	}

	boot0002, ok := mockvars.store[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "Boot0002"}]
	if !ok {
		t.Fatal("Variable Boot0002 does not exist")
	}

	if want := efi.AttributeNonVolatile | efi.AttributeBootserviceAccess | efi.AttributeRuntimeAccess; want != boot0002.attrs {
		t.Fatalf("Expected attributes %v, got %v", want, boot0002.attrs)
	}
	optGot, err = efi.ReadLoadOption(bytes.NewReader(boot0002.data))
	if err != nil {
		t.Fatalf("Cannot decode load option: %v", err)
	}
	descGot = optGot.Description

	if want := "desc2"; want != descGot {
		t.Fatalf("Expected desc %v, got %v", want, descGot)
	}
	// This is our mock path
	pathGot = optGot.FilePath
	if want := (efi.DevicePath{efi.NewFilePathDevicePathNode("path2")}); !reflect.DeepEqual(want, pathGot) {
		t.Fatalf("Expected path %v, got %v", want, pathGot)
	}

	// Check that the existing entry is not recreated
	got, err = bm.FindOrCreateEntry(BootEntry{Filename: "path2", Label: "desc2", Options: "arg3 arg4"}, "")
	if want := 2; got != want {
		t.Fatalf("expected to create Boot%04X, created Boot%04X", want, got)
	}
	if err != nil {
		t.Fatalf("could not create next boot entry, error: %v", err)
	}

}

func TestBootManagerDeleteEntry(t *testing.T) {
	mockvars := MockEFIVariables{
		map[efi.VariableDescriptor]mockEFIVariable{
			{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{1, 0, 2, 0, 3, 0}, 123},
			{GUID: efi.GlobalVariable, Name: "Boot0001"}:  {UsbrBootCdromOptBytes, 42},
			{GUID: efi.GlobalVariable, Name: "Boot0002"}:  {UsbrBootCdromOptBytes, 43},
		},
	}

	bm, err := NewBootManagerForVariables(&mockvars)
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}

	if err := bm.DeleteEntry(1); err != nil {
		t.Errorf("Expected successful deletion, got %v", err)
	}

	if !reflect.DeepEqual(bm.bootOrder, []int{2, 3}) {
		t.Errorf("Expected boot order to be 2, 3 got %v", bm.bootOrder)

	}
	if !bytes.Equal(mockvars.store[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "BootOrder"}].data, []byte{1, 0, 2, 0, 3, 0}) {
		t.Errorf("Expected actual boot order to not be changed, got %v.", mockvars.store[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "BootOrder"}])
	}
	if err := bm.DeleteEntry(1); err == nil {
		t.Errorf("Expected failure in deletion")
	}

	delete(mockvars.store, efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "Boot0002"})
	if err := bm.DeleteEntry(2); err == nil {
		t.Errorf("Expected failure in deletion")
	}
}
func TestBootManagerSetBootOrder(t *testing.T) {
	mockvars := MockEFIVariables{
		map[efi.VariableDescriptor]mockEFIVariable{
			{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{1, 0, 2, 0, 3, 0}, 123},
			{GUID: efi.GlobalVariable, Name: "Boot0001"}:  {UsbrBootCdromOptBytes, 42},
			{GUID: efi.GlobalVariable, Name: "Boot0002"}:  {UsbrBootCdromOptBytes, 43},
		},
	}
	bm, err := NewBootManagerForVariables(&mockvars)
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}

	if err := bm.PrependAndSetBootOrder([]int{2}); err != nil {
		t.Errorf("Failed to commit boot order: %v", err)
	}
	if !reflect.DeepEqual(bm.bootOrder, []int{2, 1}) {
		t.Errorf("Expected boot order to be 2, 1 got %v", bm.bootOrder)

	}
	if !bytes.Equal(mockvars.store[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "BootOrder"}].data, []byte{2, 0, 1, 0}) {
		t.Errorf("Expected actual boot order to not be changed, got %v.", mockvars.store[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "BootOrder"}])
	}
}

func TestBootManager_json(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	afero.WriteFile(memFs, "/boot/efi/path", []byte("file a"), 0644)
	afero.WriteFile(memFs, "/boot/efi/path2", []byte("file b"), 0644)
	mockvars := MockEFIVariables{
		map[efi.VariableDescriptor]mockEFIVariable{
			{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{}, efi.AttributeNonVolatile | efi.AttributeBootserviceAccess | efi.AttributeRuntimeAccess},
		},
	}

	bm, err := NewBootManagerForVariables(&mockvars)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// This creates entry Boot0000
	got, err := bm.FindOrCreateEntry(BootEntry{Filename: "/boot/efi/path", Label: "desc", Options: "arg1 arg2"}, "")
	boot0000, ok := mockvars.store[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "Boot0000"}]
	if !ok {
		t.Fatal("Variable Boot0000 does not exist")
	}

	if want := efi.AttributeNonVolatile | efi.AttributeBootserviceAccess | efi.AttributeRuntimeAccess; want != boot0000.attrs {
		t.Fatalf("Expected attributes %v, got %v", want, boot0000.attrs)
	}
	optGot, err := efi.ReadLoadOption(bytes.NewReader(boot0000.data))
	if err != nil {
		t.Fatalf("Cannot decode load option: %v", err)
	}
	descGot := optGot.Description
	if want := "desc"; want != descGot {
		t.Fatalf("Expected desc %v, got %v", want, descGot)
	}
	// This is our mock path
	pathGot := optGot.FilePath
	if want := (efi.DevicePath{efi.NewFilePathDevicePathNode("/path")}); !reflect.DeepEqual(want, pathGot) {
		t.Fatalf("Expected path %v, got %v", want, pathGot)
	}

	// This creates entry Boot0001
	got, err = bm.FindOrCreateEntry(BootEntry{Filename: "/boot/efi/path2", Label: "desc2", Options: "arg3 arg4"}, "")
	if want := 1; got != want {
		t.Fatalf("expected to create Boot%04X, created Boot%04X", want, got)
	}
	if err != nil {
		t.Fatalf("could not create next boot entry, error: %v", err)
	}

	boot0001, ok := mockvars.store[efi.VariableDescriptor{GUID: efi.GlobalVariable, Name: "Boot0001"}]
	if !ok {
		t.Fatal("Variable Boot0002 does not exist")
	}

	if want := efi.AttributeNonVolatile | efi.AttributeBootserviceAccess | efi.AttributeRuntimeAccess; want != boot0001.attrs {
		t.Fatalf("Expected attributes %v, got %v", want, boot0001.attrs)
	}
	optGot, err = efi.ReadLoadOption(bytes.NewReader(boot0001.data))
	if err != nil {
		t.Fatalf("Cannot decode load option: %v", err)
	}
	descGot = optGot.Description

	if want := "desc2"; want != descGot {
		t.Fatalf("Expected desc %v, got %v", want, descGot)
	}
	// This is our mock path
	pathGot = optGot.FilePath
	if want := (efi.DevicePath{efi.NewFilePathDevicePathNode("/path2")}); !reflect.DeepEqual(want, pathGot) {
		t.Fatalf("Expected path %v, got %v", want, pathGot)
	}
	if err := bm.PrependAndSetBootOrder([]int{0, 1}); err != nil {
		t.Errorf("Failed to commit boot order: %v", err)
	}
	if !reflect.DeepEqual(bm.bootOrder, []int{0, 1}) {
		t.Errorf("Expected boot order to be 0, 1 got %v", bm.bootOrder)
	}

	jsonBytes, err := mockvars.JSON()
	if err != nil {
		t.Fatalf("Expected JSON, received err %v", err)
	}

	want := map[string]map[string]string{
		"Boot0000": {
			"attributes": "BwA=",
			"guid":       "Yd/ki8qT0hGqDQDgmAMrjA==",
			"value":      "AQAAABQAZABlAHMAYwAAAAQEEABcAHAAYQB0AGgAAAB//wQAYQByAGcAMQAgAGEAcgBnADIAAAA=",
		},
		"Boot0001": {
			"attributes": "BwA=",
			"guid":       "Yd/ki8qT0hGqDQDgmAMrjA==",
			"value":      "AQAAABYAZABlAHMAYwAyAAAABAQSAFwAcABhAHQAaAAyAAAAf/8EAGEAcgBnADMAIABhAHIAZwA0AAAA",
		},
		"BootOrder": {
			"attributes": "BwA=",
			"guid":       "Yd/ki8qT0hGqDQDgmAMrjA==",
			"value":      "AAABAA==",
		},
	}

	gotJSON := make(map[string]map[string]string)

	if err := json.Unmarshal(jsonBytes, &gotJSON); err != nil {
		t.Fatalf("Unable to unmarshal JSON: %v", err)
	}

	if !reflect.DeepEqual(want, gotJSON) {
		t.Fatalf("Expected\n%v\ngot\n%v\nerr: %v", want, gotJSON, err)
	}
}

func TestBootManager_unsupported(t *testing.T) {
	mockvars := NoEFIVariables{}

	_, err := NewBootManagerForVariables(&mockvars)

	if err == nil {
		t.Fatalf("Unexpected success")
	}

	if err.Error() != "Variables not supported" {
		t.Fatalf("Unexpected error: %v", err)
	}
}
