// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/afero/mem"

	"gopkg.in/check.v1"
)

type MapFS struct {
	p afero.Fs
}

type dirEntry struct {
	os.FileInfo
}

func (d dirEntry) Info() (os.FileInfo, error) { return os.FileInfo(d), nil }
func (d dirEntry) Type() os.FileMode          { return d.Mode().Type() }

func (m MapFS) Create(path string) (File, error)             { return m.p.Create(path) }
func (m MapFS) MkdirAll(path string, perm os.FileMode) error { return m.p.MkdirAll(path, perm) }
func (m MapFS) Open(path string) (File, error)               { return m.p.Open(path) }
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
func (m MapFS) Readlink(path string) (target string, err error) {
	defer func() {
		fmt.Println("readlink path:", path, "target:", target, "err:", err)
		var e *os.PathError
		if errors.As(err, &e) && e.Op != "readlink" {
			err = &os.PathError{Op: "readlink", Path: path, Err: e.Err}
		}
	}()

	fi, err := m.p.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return "", &os.PathError{Op: "readlink", Path: path, Err: syscall.EINVAL}
	}
	tgt, err := afero.ReadFile(m.p, path)
	return string(tgt), err

}
func (m MapFS) Remove(path string) error                  { return m.p.Remove(path) }
func (m MapFS) Rename(oldname, newname string) error      { return m.p.Rename(oldname, newname) }
func (m MapFS) Stat(path string) (os.FileInfo, error)     { return m.p.Stat(path) }
func (m MapFS) TempFile(dir, prefix string) (File, error) { return afero.TempFile(m.p, dir, prefix) }

type mapFsMixin struct {
	restoreFs func()
	fs        afero.Afero
}

func (m *mapFsMixin) SetUpTest(c *check.C) {
	m.restoreFs = m.mockFs(afero.NewMemMapFs())
}

func (m *mapFsMixin) TearDownTest(c *check.C) {
	if m.restoreFs != nil {
		m.restoreFs()
		m.restoreFs = nil
	}
}

func (m *mapFsMixin) mockFs(fs afero.Fs) (restore func()) {
	origAppFs := appFs
	origMockFs := m.fs

	appFs = MapFS{fs}
	m.fs = afero.Afero{Fs: fs}

	return func() {
		appFs = origAppFs
		m.fs = origMockFs
	}
}

// writeFile writes n repeatable sequences of blockSz bytes to path. Select blockSz
// as a prime number to reduce the number of repeated blocks.
func (m *mapFsMixin) writeFile(c *check.C, path string, firstByte, blockSz uint8, n int) {
	data := make([]byte, 0, blockSz)
	for i := uint8(0); i < blockSz; i++ {
		data = append(data, i+firstByte)
	}
	w := new(bytes.Buffer)
	for ; n > 0; n-- {
		w.Write(data)
	}
	c.Check(m.fs.WriteFile(path, w.Bytes(), 0644), check.IsNil)
}

func (m *mapFsMixin) symlink(c *check.C, oldname, newname string) {
	f, err := m.fs.OpenFile(newname, os.O_WRONLY|os.O_CREATE, 0777)
	c.Assert(err, check.IsNil)
	defer f.Close()

	_, err = io.WriteString(f, oldname)
	c.Check(err, check.IsNil)

	mem.SetMode(f.(*mem.File).Data(), os.ModeSymlink|0777)
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

	// XXX: Need to check that updating is done in an atomic way,
	// possibly by verifying that it's a new file. But that needs
	// a real filesystem.
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
