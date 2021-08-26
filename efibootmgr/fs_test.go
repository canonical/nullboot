// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"errors"
	"github.com/spf13/afero"
	"io"
	"os"
	"testing"
)

type MapFS struct {
	p afero.Fs
}

type dirEntry struct {
	os.FileInfo
}

func (d dirEntry) Info() (os.FileInfo, error) { return os.FileInfo(d), nil }
func (d dirEntry) Type() os.FileMode          { return d.Mode().Type() }

func (m MapFS) Create(path string) (io.WriteCloser, error)  { return m.p.Create(path) }
func (m MapFS) Open(path string) (io.ReadSeekCloser, error) { return m.p.Open(path) }
func (m MapFS) Remove(path string) error                    { return m.p.Remove(path) }
func (m MapFS) ReadDir(path string) ([]os.DirEntry, error) {
	var out []os.DirEntry
	fis, err := afero.ReadDir(m.p, path)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		out = append(out, dirEntry{fi})
	}
	return out, nil
}

func TestMaybeUpdateFile_missingSrc(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	updated, err := MaybeUpdateFile("dst", "src")
	if err == nil {
		t.Errorf("Expected error")
	}
	if updated {
		t.Errorf("File was unexpectedly updated")
	}
	if _, err := memFs.Stat("dst"); !os.IsNotExist(err) {
		t.Errorf("file \"%s\" exists or something\n", "dst")
	}
	if _, err := memFs.Stat("src"); !os.IsNotExist(err) {
		t.Errorf("file \"%s\" exists or something\n", "src")
	}
}

func TestMaybeUpdateFile_newFile(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	afero.WriteFile(memFs, "src", []byte("file b"), 0644)
	updated, err := MaybeUpdateFile("dst", "src")
	if err != nil {
		t.Errorf("Could not update file: %v", err)
	}
	if !updated {
		t.Errorf("Did not update")
	}

	srcBytes, err := afero.ReadFile(memFs, "src")
	if err != nil {
		t.Errorf("Could not read src: %v", err)
	}
	dstBytes, err := afero.ReadFile(memFs, "dst")
	if err != nil {
		t.Errorf("Could not read dst: %v", err)
	}
	if !bytes.Equal(srcBytes, dstBytes) {
		t.Errorf("Expected: %v, got: %v", srcBytes, dstBytes)
	}
}

func TestMaybeUpdateFile_updateFile(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	afero.WriteFile(memFs, "src", []byte("file b"), 0644)
	afero.WriteFile(memFs, "dst", []byte("file a"), 0644)
	updated, err := MaybeUpdateFile("dst", "src")
	if err != nil {
		t.Errorf("Could not update file: %v", err)
	}
	if !updated {
		t.Errorf("Did not update")
	}

	srcBytes, err := afero.ReadFile(memFs, "src")
	if err != nil {
		t.Errorf("Could not read src: %v", err)
	}
	dstBytes, err := afero.ReadFile(memFs, "dst")
	if err != nil {
		t.Errorf("Could not read dst: %v", err)
	}
	if !bytes.Equal(srcBytes, dstBytes) {
		t.Errorf("Expected: %v, got: %v", srcBytes, dstBytes)
	}
}

func TestMaybeUpdateFile_readOnlyTarget(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	afero.WriteFile(memFs, "src", []byte("file b"), 0644)
	afero.WriteFile(memFs, "dst", []byte("file a"), 0644)
	appFs = MapFS{afero.NewReadOnlyFs(memFs)}
	updated, err := MaybeUpdateFile("dst", "src")
	if err == nil {
		t.Errorf("Expected errro")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("Expected to fail with permission error, got: %v", err)
	}
	if updated {
		t.Errorf("Expected not to have updated, but somehow did")
	}
}

func TestMaybeUpdateFile_sameFile(t *testing.T) {
	memFs := afero.NewMemMapFs()
	appFs = MapFS{memFs}
	afero.WriteFile(memFs, "src", []byte("file b"), 0644)
	afero.WriteFile(memFs, "dst", []byte("file b"), 0644)
	appFs = MapFS{afero.NewReadOnlyFs(memFs)}
	updated, err := MaybeUpdateFile("dst", "src")
	if err != nil {
		t.Errorf("Could not update file: %v", err)
	}
	if updated {
		t.Errorf("Rewrote existing file")
	}

	if _, err := memFs.Stat("dst"); os.IsNotExist(err) {
		t.Errorf("file \"%s\" does not exist.\n", "dst")
	}
}
