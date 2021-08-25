// This file is part of bootmgrless
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"github.com/canonical/bootmgrless/efivars"
	"reflect"
	"testing"
)

func TestBootManager_mocked(t *testing.T) {
	mockvars := MockEFIVariables{
		map[efivars.GUID]map[string]mockEFIVariable{
			efivars.GUIDGlobal: {
				"Boot0001": {UsbrBootCdrom, 42},
			},
		},
	}

	bm, err := newBootManagerFromVariables(&mockvars)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(bm.entries) != 1 {
		t.Fatalf("Could not parse an entry")
	}

	want := BootEntryVariable{1, UsbrBootCdrom, 42, efivars.LoadOption{Data: UsbrBootCdrom}}
	if !reflect.DeepEqual(bm.entries[1], want) {
		t.Fatalf("\n"+
			"expected: %+v\n"+
			"got:      %+v", want, bm.entries[1])
	}

	// This creates entry Boot0000
	got, err := bm.AddEntry("desc", "path", []string{"arg1", "arg2"})
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
	got, err = bm.AddEntry("desc2", "path2", []string{"arg3", "arg4"})
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

}

func TestBootManager_unsupported(t *testing.T) {
	mockvars := NoEFIVariables{}

	_, err := newBootManagerFromVariables(&mockvars)

	if err == nil {
		t.Fatalf("Unexpected success")
	}

	if err.Error() != "Variables not supported" {
		t.Fatalf("Unexpected error: %v", err)
	}
}
