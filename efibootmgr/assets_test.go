// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"crypto"

	"gopkg.in/check.v1"
)

type assetsSuite struct {
	mapFsMixin
}

var _ = check.Suite(&assetsSuite{})

func (s *assetsSuite) TestNewTrustedAssets(c *check.C) {
	assets := newTrustedAssets()
	c.Check(assets, check.NotNil)
	c.Check(assets.loaded.Alg, check.Equals, hashAlg{Hash: crypto.SHA256})
	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte(nil))
	c.Check(assets.newAssets, check.DeepEquals, [][]byte(nil))
}

func (s *assetsSuite) TestReadTrustedAssets(c *check.C) {
	payload := []byte(`
{
	"alg": "sha256",
	"hashes": [
		"tbudgBSg+bHWHiHnlteNzN8TUvI80ygS9IULh4rklEw=",
		"fYZelZskZpGMmGOvypQtD7idfJrAyZuvw3SVBN7ZdzA="
	]
}`)
	c.Check(s.fs.WriteFile(trustedAssetsPath, payload, 0644), check.IsNil)

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)
	c.Check(assets.loaded.Alg, check.Equals, hashAlg{Hash: crypto.SHA256})
	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"),
		decodeHexString(c, "7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730"),
	})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte(nil))
}

func (s *assetsSuite) TestReadTrustedAssetsNoFile(c *check.C) {
	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)
	c.Check(assets.loaded.Alg, check.Equals, hashAlg{Hash: crypto.SHA256})
	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte(nil))
	c.Check(assets.newAssets, check.DeepEquals, [][]byte(nil))
}

func (s *assetsSuite) TestReadTrustedAssetsMissingAlg(c *check.C) {
	payload := []byte(`
{
	"hashes": [
		"tbudgBSg+bHWHiHnlteNzN8TUvI80ygS9IULh4rklEw=",
		"fYZelZskZpGMmGOvypQtD7idfJrAyZuvw3SVBN7ZdzA="
	]
}`)
	c.Check(s.fs.WriteFile(trustedAssetsPath, payload, 0644), check.IsNil)

	_, err := ReadTrustedAssets()
	c.Assert(err, check.ErrorMatches, "digest algorithm unknown hash value 0 is not available")
}

func (s *assetsSuite) TestReadTrustedAssetsInvalidAlg(c *check.C) {
	payload := []byte(`
{
	"alg": "foo",
	"hashes": [
		"tbudgBSg+bHWHiHnlteNzN8TUvI80ygS9IULh4rklEw=",
		"fYZelZskZpGMmGOvypQtD7idfJrAyZuvw3SVBN7ZdzA="
	]
}`)
	c.Check(s.fs.WriteFile(trustedAssetsPath, payload, 0644), check.IsNil)

	_, err := ReadTrustedAssets()
	c.Assert(err, check.ErrorMatches, "unsupported hash algorithm: foo")
}

func (s *assetsSuite) TestTrustNewFromDir(c *check.C) {
	// Write some files with a repeating payload to test file hashing - the
	// payload size is selected to not repeat on block boundaries and not
	// produce a total size that is a multiple of the block size (to test
	// the padding behaviour).

	// Write a file that is just under 10 blocks long to produce a hash tree
	// with a depth of 2.
	s.writeFile(c, "/foo/1", 0, 199, 200)

	// Write a file that is just over 170 blocks long to produce a hash tree
	// with a depth of 3. The middle level of the tree has 2 blocks, with the
	// last block only partially filled.
	s.writeFile(c, "/foo/2", 0, 199, 3500)

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	assets.loaded.Hashes = [][]byte{
		decodeHexString(c, "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"),
		decodeHexString(c, "7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730"),
	}

	c.Check(assets.TrustNewFromDir("/foo"), check.IsNil)

	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"),
		decodeHexString(c, "7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730"),
		decodeHexString(c, "73e60cb7e2d9c8ba47a507c647f9b388900f5a5dc33c24d4a95f84f4dd85dcec"),
		decodeHexString(c, "6c05c5017b4e584ce0e4e77b42e7399c0392407216803f24233def5c038adc7c"),
	})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte{
		decodeHexString(c, "73e60cb7e2d9c8ba47a507c647f9b388900f5a5dc33c24d4a95f84f4dd85dcec"),
		decodeHexString(c, "6c05c5017b4e584ce0e4e77b42e7399c0392407216803f24233def5c038adc7c"),
	})
}

func (s *assetsSuite) TestTrustNewFromDirDeDup(c *check.C) {
	c.Check(s.fs.WriteFile("/foo/1", []byte("some contents"), 0644), check.IsNil)

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	assets.loaded.Hashes = [][]byte{
		decodeHexString(c, "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"),
		decodeHexString(c, "7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730"),
		decodeHexString(c, "8c3bb60fb858eccd3e85ba8fd3a85d9014f468defbdf6bc0c46891b2049eca46"),
	}

	c.Check(assets.TrustNewFromDir("/foo"), check.IsNil)

	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"),
		decodeHexString(c, "7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730"),
		decodeHexString(c, "8c3bb60fb858eccd3e85ba8fd3a85d9014f468defbdf6bc0c46891b2049eca46"),
	})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte{
		decodeHexString(c, "8c3bb60fb858eccd3e85ba8fd3a85d9014f468defbdf6bc0c46891b2049eca46"),
	})
}

func (s *assetsSuite) TestRemoveObsolete(c *check.C) {
	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	assets.loaded.Hashes = [][]byte{
		decodeHexString(c, "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"),
		decodeHexString(c, "7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730"),
		decodeHexString(c, "73e60cb7e2d9c8ba47a507c647f9b388900f5a5dc33c24d4a95f84f4dd85dcec"),
		decodeHexString(c, "6c05c5017b4e584ce0e4e77b42e7399c0392407216803f24233def5c038adc7c"),
	}
	assets.newAssets = [][]byte{
		decodeHexString(c, "73e60cb7e2d9c8ba47a507c647f9b388900f5a5dc33c24d4a95f84f4dd85dcec"),
		decodeHexString(c, "6c05c5017b4e584ce0e4e77b42e7399c0392407216803f24233def5c038adc7c"),
		decodeHexString(c, "6c05c5017b4e584ce0e4e77b42e7399c0392407216803f24233def5c038adc7c"),
	}

	assets.RemoveObsolete()

	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "73e60cb7e2d9c8ba47a507c647f9b388900f5a5dc33c24d4a95f84f4dd85dcec"),
		decodeHexString(c, "6c05c5017b4e584ce0e4e77b42e7399c0392407216803f24233def5c038adc7c"),
	})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte{
		decodeHexString(c, "73e60cb7e2d9c8ba47a507c647f9b388900f5a5dc33c24d4a95f84f4dd85dcec"),
		decodeHexString(c, "6c05c5017b4e584ce0e4e77b42e7399c0392407216803f24233def5c038adc7c"),
		decodeHexString(c, "6c05c5017b4e584ce0e4e77b42e7399c0392407216803f24233def5c038adc7c"),
	})
}

func (s *assetsSuite) TestSave(c *check.C) {
	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	assets.loaded.Hashes = [][]byte{
		decodeHexString(c, "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"),
		decodeHexString(c, "7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730"),
		decodeHexString(c, "73e60cb7e2d9c8ba47a507c647f9b388900f5a5dc33c24d4a95f84f4dd85dcec"),
		decodeHexString(c, "6c05c5017b4e584ce0e4e77b42e7399c0392407216803f24233def5c038adc7c"),
	}

	c.Check(assets.Save(), check.IsNil)

	data, err := s.fs.ReadFile(trustedAssetsPath)
	c.Check(err, check.IsNil)
	c.Check(data, check.DeepEquals, []byte(`{"alg":"sha256","hashes":["tbudgBSg+bHWHiHnlteNzN8TUvI80ygS9IULh4rklEw=","fYZelZskZpGMmGOvypQtD7idfJrAyZuvw3SVBN7ZdzA=","c+YMt+LZyLpHpQfGR/mziJAPWl3DPCTUqV+E9N2F3Ow=","bAXFAXtOWEzg5Od7Quc5nAOSQHIWgD8kIz3vXAOK3Hw="]}
`))
}
