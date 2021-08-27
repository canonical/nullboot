// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"github.com/canonical/nullboot/efivars"
	"reflect"
	"testing"
)

func TestBootManager_mocked(t *testing.T) {
	mockvars := MockEFIVariables{
		map[efivars.GUID]map[string]mockEFIVariable{
			efivars.GUIDGlobal: {
				"BootOrder": {[]byte{1, 0, 2, 0, 3, 0}, 123},
				"Boot0001":  {UsbrBootCdrom, 42},
			},
		},
	}

	appEFIVars = &mockvars
	bm, err := NewBootManagerFromSystem()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(bm.entries) != 1 {
		t.Fatalf("Could not parse an entry")
	}

	if want := []int{1, 2, 3}; !reflect.DeepEqual(bm.bootOrder, want) {
		t.Fatalf("Expected %v, got: %v", want, bm.bootOrder)
	}

	want := BootEntryVariable{1, UsbrBootCdrom, 42, efivars.LoadOption{Data: UsbrBootCdrom}}
	if !reflect.DeepEqual(bm.entries[1], want) {
		t.Fatalf("\n"+
			"expected: %+v\n"+
			"got:      %+v", want, bm.entries[1])
	}

	// This creates entry Boot0000
	got, err := bm.FindOrCreateEntry(BootEntry{Filename: "path", Label: "desc", Options: "arg1 arg2"})
	if want := 0; got != want {
		t.Fatalf("expected to create Boot%04X, created Boot%04X", want, got)
	}
	if err != nil {
		t.Fatalf("could not create next boot entry, error: %v", err)
	}

	boot0000, ok := mockvars.store[efivars.GUIDGlobal]["Boot0000"]
	if !ok {
		t.Fatal("Variable Boot0000 does not exist")
	}

	if want := uint32(efivars.VariableNonVolatile) | efivars.VariableBootServiceAccess | efivars.VariableRuntimeAccess; want != boot0000.attrs {
		t.Fatalf("Expected attributes %v, got %v", want, boot0000.attrs)
	}
	descGot := efivars.LoadOption{Data: boot0000.data}.Desc()
	if want := "desc"; want != descGot {
		t.Fatalf("Expected desc %v, got %v", want, descGot)
	}
	// This is our mock path
	pathGot := efivars.LoadOption{Data: boot0000.data}.Path()
	if want := (efivars.LoadOption{Data: UsbrBootCdrom}).Path(); !bytes.Equal(want, pathGot) {
		t.Fatalf("Expected path %v, got %v", want, pathGot)
	}

	// This creates entry Boot0002
	got, err = bm.FindOrCreateEntry(BootEntry{Filename: "path2", Label: "desc2", Options: "arg3 arg4"})
	if want := 2; got != want {
		t.Fatalf("expected to create Boot%04X, created Boot%04X", want, got)
	}
	if err != nil {
		t.Fatalf("could not create next boot entry, error: %v", err)
	}

	boot0002, ok := mockvars.store[efivars.GUIDGlobal]["Boot0002"]
	if !ok {
		t.Fatal("Variable Boot0002 does not exist")
	}

	if want := uint32(efivars.VariableNonVolatile) | efivars.VariableBootServiceAccess | efivars.VariableRuntimeAccess; want != boot0002.attrs {
		t.Fatalf("Expected attributes %v, got %v", want, boot0002.attrs)
	}
	descGot = efivars.LoadOption{Data: boot0002.data}.Desc()
	if want := "desc2"; want != descGot {
		t.Fatalf("Expected desc %v, got %v", want, descGot)
	}
	// This is our mock path
	pathGot = efivars.LoadOption{Data: boot0002.data}.Path()
	if want := (efivars.LoadOption{Data: UsbrBootCdrom}).Path(); !bytes.Equal(want, pathGot) {
		t.Fatalf("Expected path %v, got %v", want, pathGot)
	}

	// Check that the existing entry is not recreated
	got, err = bm.FindOrCreateEntry(BootEntry{Filename: "path2", Label: "desc2", Options: "arg3 arg4"})
	if want := 2; got != want {
		t.Fatalf("expected to create Boot%04X, created Boot%04X", want, got)
	}
	if err != nil {
		t.Fatalf("could not create next boot entry, error: %v", err)
	}

}

func TestBootManagerDeleteEntry(t *testing.T) {
	mockvars := MockEFIVariables{
		map[efivars.GUID]map[string]mockEFIVariable{
			efivars.GUIDGlobal: {
				"BootOrder": {[]byte{1, 0, 2, 0, 3, 0}, 123},
				"Boot0001":  {UsbrBootCdrom, 42},
				"Boot0002":  {UsbrBootCdrom, 43},
			},
		},
	}

	appEFIVars = &mockvars
	bm, err := NewBootManagerFromSystem()
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}

	if err := bm.DeleteEntry(1); err != nil {
		t.Errorf("Expected successful deletion, got %v", err)
	}

	if !reflect.DeepEqual(bm.bootOrder, []int{2, 3}) {
		t.Errorf("Expected boot order to be 2, 3 got %v", bm.bootOrder)

	}
	if !bytes.Equal(mockvars.store[efivars.GUIDGlobal]["BootOrder"].data, []byte{1, 0, 2, 0, 3, 0}) {
		t.Errorf("Expected actual boot order to not be changed, got %v.", mockvars.store[efivars.GUIDGlobal]["BootOrder"])
	}
	if err := bm.DeleteEntry(1); err == nil {
		t.Errorf("Expected failure in deletion")
	}

	delete(mockvars.store[efivars.GUIDGlobal], "Boot0002")
	if err := bm.DeleteEntry(2); err == nil {
		t.Errorf("Expected failure in deletion")
	}
}
func TestBootManagerSetBootOrder(t *testing.T) {
	mockvars := MockEFIVariables{
		map[efivars.GUID]map[string]mockEFIVariable{
			efivars.GUIDGlobal: {
				"BootOrder": {[]byte{1, 0, 2, 0, 3, 0}, 123},
				"Boot0001":  {UsbrBootCdrom, 42},
				"Boot0002":  {UsbrBootCdrom, 43},
			},
		},
	}
	appEFIVars = &mockvars
	bm, err := NewBootManagerFromSystem()
	if err != nil {
		t.Fatalf("Could not create boot manager: %v", err)
	}

	if err := bm.PrependAndSetBootOrder([]int{2}); err != nil {
		t.Errorf("Failed to commit boot order: %v", err)
	}
	if !reflect.DeepEqual(bm.bootOrder, []int{2, 1}) {
		t.Errorf("Expected boot order to be 2, 1 got %v", bm.bootOrder)

	}
	if !bytes.Equal(mockvars.store[efivars.GUIDGlobal]["BootOrder"].data, []byte{2, 0, 1, 0}) {
		t.Errorf("Expected actual boot order to not be changed, got %v.", mockvars.store[efivars.GUIDGlobal]["BootOrder"])
	}
}
func TestBootManager_unsupported(t *testing.T) {
	mockvars := NoEFIVariables{}

	appEFIVars = &mockvars
	_, err := NewBootManagerFromSystem()

	if err == nil {
		t.Fatalf("Unexpected success")
	}

	if err.Error() != "Variables not supported" {
		t.Fatalf("Unexpected error: %v", err)
	}
}
