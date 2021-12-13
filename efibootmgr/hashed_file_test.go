// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"crypto"
	"errors"
	"io"

	"gopkg.in/check.v1"
)

type hashedFileSuite struct {
	mapFsMixin
}

var _ = check.Suite(&hashedFileSuite{})

type testHashedFileReadBlock struct {
	off int64
	sz  int64
	n   int64
}

func (s *hashedFileSuite) testHashedFile(c *check.C, path string, blocks []testHashedFileReadBlock) {
	f, err := appFs.Open(path)
	c.Assert(err, check.IsNil)
	defer f.Close()

	var leafHashes [][]byte

	for {
		var data [hashBlockSize]byte
		n, err := f.Read(data[:])
		if n > 0 {
			h := crypto.SHA256.New()
			h.Write(data[:])
			leafHashes = append(leafHashes, h.Sum(nil))
		}
		if err != nil {
			break
		}
	}
	h := crypto.SHA256.New()
	for _, leaf := range leafHashes {
		h.Write(leaf)
	}
	expectedHash := h.Sum(nil)
	leafHashes = nil

	hf, err := newHashedFile(f, crypto.SHA256, func(hashes [][]byte) {
		leafHashes = hashes
	})
	c.Assert(err, check.IsNil)

	for _, block := range blocks {
		total := block.sz * block.n

		expected := make([]byte, total)
		data := make([]byte, total)

		for i := int64(0); i < total; {
			n, err := f.ReadAt(expected[i:i+block.sz], i+block.off)
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) || int64(n) < block.sz {
				break
			}
			c.Check(err, check.IsNil)
			i += int64(n)
		}

		for i := int64(0); i < total; {
			n, err := hf.ReadAt(data[i:i+block.sz], i+block.off)
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) || int64(n) < block.sz {
				break
			}
			c.Check(err, check.IsNil)
			i += int64(n)
		}

		c.Check(data, check.DeepEquals, expected)
	}

	c.Check(hf.Close(), check.IsNil)

	h = crypto.SHA256.New()
	for _, leaf := range leafHashes {
		h.Write(leaf)
	}
	c.Check(h.Sum(nil), check.DeepEquals, expectedHash)
}

func (s *hashedFileSuite) TestHashedFileReadFullSmallReads(c *check.C) {
	s.writeFile(c, "/foo", 0, 199, 3500)
	s.testHashedFile(c, "/foo", []testHashedFileReadBlock{
		{off: 0, sz: 10, n: 69650},
	})
}

func (s *hashedFileSuite) TestHashedFileReadFullLargeReads(c *check.C) {
	s.writeFile(c, "/foo", 0, 199, 3500)
	s.testHashedFile(c, "/foo", []testHashedFileReadBlock{
		{off: 0, sz: 69650, n: 10},
	})
}

func (s *hashedFileSuite) TestHashedFileReadSparse(c *check.C) {
	s.writeFile(c, "/foo", 0, 199, 3500)
	s.testHashedFile(c, "/foo", []testHashedFileReadBlock{
		{off: 500, sz: 10, n: 100},
		{off: 20000, sz: 500, n: 20},
	})
}
