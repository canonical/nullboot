// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// FS abstracts away the filesystem.
//
// So we really wanted to use afero because it does all the magic for us, but it doubles
// our binary size, so that seems a tad much.
type FS interface {
	// Create behaves like os.Create()
	Create(path string) (io.WriteCloser, error)
	// MkdirAll behaves like os.MkdirAll()
	MkdirAll(path string, perm os.FileMode) error
	// Open behaves like os.Open()
	Open(path string) (io.ReadSeekCloser, error)
	// ReadDir behaves like os.ReadDir()
	ReadDir(path string) ([]os.DirEntry, error)
	// Remove behaves like os.Remove()
	Remove(path string) error
}

// realFS implements FS using the os package
type realFS struct{}

func (realFS) Create(path string) (io.WriteCloser, error)   { return os.Create(path) }
func (realFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (realFS) Open(path string) (io.ReadSeekCloser, error)  { return os.Open(path) }
func (realFS) ReadDir(path string) ([]os.DirEntry, error)   { return os.ReadDir(path) }
func (realFS) Remove(path string) error                     { return os.Remove(path) }

// appFs is our default FS
var appFs FS = realFS{}

// MaybeUpdateFile copies src to dest if they are different
// It returns true if the destination file was successfully updated. If the return value
// is false, the state of the destination is unspecified. It might not exist, exist
// with partial data or exist with old data, amongst others.
func MaybeUpdateFile(dst string, src string) (bool, error) {
	srcFile, err := appFs.Open(src)
	if err != nil {
		return false, fmt.Errorf("Could not open source file: %w", err)
	}
	defer srcFile.Close()

	if needUpdate, err := needUpdateFile(dst, src, srcFile); !needUpdate {
		return false, err
	}

	dstFileWriter, err := appFs.Create(dst)
	if err != nil {
		return false, fmt.Errorf("Could not open %s for writing: %w", dst, err)
	}
	defer dstFileWriter.Close()
	// FIXME: Delete the file on failure

	if _, err := io.Copy(dstFileWriter, srcFile); err != nil {
		return false, fmt.Errorf("Could not copy %s to %s: %w", src, dst, err)
	}
	return true, nil
}

func needUpdateFile(dst string, src string, srcFile io.ReadSeeker) (bool, error) {
	// To keep things simple, but not have the files in memory, just hash them
	dstHash := sha256.New()
	srcHash := sha256.New()

	dstFile, err := appFs.Open(dst)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("Could not open destination file: %w", err)
	}

	defer dstFile.Close()

	if _, err := io.Copy(dstHash, dstFile); err != nil {
		return false, fmt.Errorf("Could not hash destination file %s: %w", dst, err)
	}
	if _, err := io.Copy(srcHash, srcFile); err != nil {
		return false, fmt.Errorf("Could not hash source file %s: %w", src, err)
	}
	if bytes.Equal(dstHash.Sum(nil), srcHash.Sum(nil)) {
		return false, nil
	}

	if _, err := srcFile.Seek(0, io.SeekStart); err != nil {
		return false, fmt.Errorf("Could not seek in source file %s: %w", src, err)
	}

	return true, nil
}
