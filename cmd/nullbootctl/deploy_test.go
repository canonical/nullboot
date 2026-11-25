// This file is part of nullboot
// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/go-tpm2"
	"github.com/canonical/go-tpm2/mu"
	"github.com/canonical/go-tpm2/objectutil"

	"gopkg.in/check.v1"

	"github.com/canonical/nullboot/internal/gpt"
)

// Test hooks gocheck into go test for this package.
func Test(t *testing.T) { check.TestingT(t) }

type deploySuite struct{}

var _ = check.Suite(&deploySuite{})

// writeMuFile marshals v using mu.MarshalToWriter and writes the result to
// path, mirroring the format produced by tools such as tpm2_readpublic for
// TPM2B-wrapped structures.
func writeMuFile(c *check.C, path string, v interface{}) {
	f, err := os.Create(path)
	c.Assert(err, check.IsNil)
	defer f.Close()

	_, err = mu.MarshalToWriter(f, v)
	c.Assert(err, check.IsNil)
}

// newTestSRKPublic returns a valid RSA storage key template suitable for use
// as a fixture in tests that need a *tpm2.Public.
func newTestSRKPublic() *tpm2.Public {
	pub := objectutil.NewRSAStorageKeyTemplate()
	pub.Unique = &tpm2.PublicIDU{RSA: make(tpm2.PublicKeyRSA, 256)}
	for i := range pub.Unique.RSA {
		pub.Unique.RSA[i] = byte(i)
	}
	return pub
}

// writeSRKPubFile writes pub to path in the TPM2B_PUBLIC-like, double
// wrapped format expected by [imageDeployer.readSRKPublicArea].
func writeSRKPubFile(c *check.C, path string, pub *tpm2.Public) {
	raw, err := mu.MarshalToBytes(pub)
	c.Assert(err, check.IsNil)
	writeMuFile(c, path, raw)
}

func (s *deploySuite) TestReadUniqueDataRSA(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "unique")

	expected := tpm2.PublicKeyRSA{0xaa, 0xbb, 0xcc, 0xdd}
	writeMuFile(c, path, expected)

	u, err := readUniqueData(path, tpm2.ObjectTypeRSA)
	c.Assert(err, check.IsNil)
	c.Assert(u, check.NotNil)
	c.Check([]byte(u.RSA), check.DeepEquals, []byte(expected))
}

func (s *deploySuite) TestReadUniqueDataECC(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "unique")

	expected := tpm2.ECCPoint{X: tpm2.ECCParameter{0x01, 0x02}, Y: tpm2.ECCParameter{0x03, 0x04}}
	writeMuFile(c, path, expected)

	u, err := readUniqueData(path, tpm2.ObjectTypeECC)
	c.Assert(err, check.IsNil)
	c.Assert(u, check.NotNil)
	c.Assert(u.ECC, check.NotNil)
	c.Check([]byte(u.ECC.X), check.DeepEquals, []byte(expected.X))
	c.Check([]byte(u.ECC.Y), check.DeepEquals, []byte(expected.Y))
}

func (s *deploySuite) TestReadUniqueDataUnsupportedType(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "unique")
	c.Assert(os.WriteFile(path, nil, 0600), check.IsNil)

	_, err := readUniqueData(path, tpm2.ObjectTypeKeyedHash)
	c.Check(err, check.ErrorMatches, "unsupported type")
}

func (s *deploySuite) TestReadUniqueDataFileNotFound(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "does-not-exist")

	_, err := readUniqueData(path, tpm2.ObjectTypeRSA)
	c.Check(err, check.ErrorMatches, "cannot open file: .*")
}

func (s *deploySuite) TestReadUniqueDataCorruptData(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "unique")
	// A truncated/invalid mu-encoded stream.
	c.Assert(os.WriteFile(path, []byte{0x00}, 0600), check.IsNil)

	_, err := readUniqueData(path, tpm2.ObjectTypeRSA)
	c.Check(err, check.ErrorMatches, "cannot unmarshal unique data: .*")
}

func (s *deploySuite) TestMaybeAddRecoveryKeyNoFileConfigured(c *check.C) {
	d := &imageDeployer{opts: &deployOptions{}}

	// With RecoveryKeyFile unset, this must return immediately without
	// touching the (unset) root device.
	c.Check(d.maybeAddRecoveryKey(nil), check.IsNil)
}

func (s *deploySuite) TestMaybeAddRecoveryKeyFileNotFound(c *check.C) {
	dir := c.MkDir()
	d := &imageDeployer{opts: &deployOptions{
		RecoveryKeyFile: filepath.Join(dir, "does-not-exist"),
	}}

	err := d.maybeAddRecoveryKey(nil)
	c.Check(err, check.ErrorMatches, "cannot read recovery key from file: .*")
}

func (s *deploySuite) TestMaybeAddRecoveryKeyWrongLength(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "recovery-key")
	c.Assert(os.WriteFile(path, []byte("too-short"), 0600), check.IsNil)

	d := &imageDeployer{opts: &deployOptions{RecoveryKeyFile: path}}

	err := d.maybeAddRecoveryKey(nil)
	c.Check(err, check.ErrorMatches, "recovery key must be 16 bytes")
}

func (s *deploySuite) TestMaybeWriteCustomSRKTemplateStandardSkipsWrite(c *check.C) {
	esp := c.MkDir()
	d := &imageDeployer{opts: &deployOptions{StandardSRKTemplate: true}}

	srkPub := newTestSRKPublic()
	c.Check(d.maybeWriteCustomSRKTemplate(esp, srkPub), check.IsNil)

	_, err := os.Stat(filepath.Join(esp, "tpm2-srk.tmpl"))
	c.Check(errors.Is(err, fs.ErrNotExist), check.Equals, true)
}

func (s *deploySuite) TestMaybeWriteCustomSRKTemplateDefault(c *check.C) {
	esp := c.MkDir()
	d := &imageDeployer{opts: &deployOptions{}}

	srkPub := newTestSRKPublic()
	c.Assert(d.maybeWriteCustomSRKTemplate(esp, srkPub), check.IsNil)

	tmpl := readTemplate(c, esp)
	c.Check(tmpl.Type, check.Equals, srkPub.Type)
	c.Check(tmpl.NameAlg, check.Equals, srkPub.NameAlg)
	c.Check(tmpl.Attrs, check.Equals, srkPub.Attrs)
	// The template's unique data is cleared, so after the TPM2B round trip
	// it decodes as an empty (not the original, populated) RSA value.
	c.Assert(tmpl.Unique, check.NotNil)
	c.Check(len(tmpl.Unique.RSA), check.Equals, 0)
}

func (s *deploySuite) TestMaybeWriteCustomSRKTemplateWithUniqueData(c *check.C) {
	esp := c.MkDir()

	uniquePath := filepath.Join(c.MkDir(), "unique")
	rsaUnique := tpm2.PublicKeyRSA{0x11, 0x22, 0x33}
	writeMuFile(c, uniquePath, rsaUnique)

	d := &imageDeployer{opts: &deployOptions{SRKTemplateUniqueData: uniquePath}}

	srkPub := newTestSRKPublic()
	c.Assert(d.maybeWriteCustomSRKTemplate(esp, srkPub), check.IsNil)

	tmpl := readTemplate(c, esp)
	c.Assert(tmpl.Unique, check.NotNil)
	c.Check([]byte(tmpl.Unique.RSA), check.DeepEquals, []byte(rsaUnique))
}

func (s *deploySuite) TestMaybeWriteCustomSRKTemplateBadUniqueDataPath(c *check.C) {
	esp := c.MkDir()
	d := &imageDeployer{opts: &deployOptions{
		SRKTemplateUniqueData: filepath.Join(c.MkDir(), "does-not-exist"),
	}}

	err := d.maybeWriteCustomSRKTemplate(esp, newTestSRKPublic())
	c.Check(err, check.ErrorMatches, "cannot read unique data: .*")
}

func (s *deploySuite) TestMaybeWriteCustomSRKTemplateCannotOpenFile(c *check.C) {
	// A non-existent ESP directory means the template file can't be created.
	esp := filepath.Join(c.MkDir(), "missing-dir")
	d := &imageDeployer{opts: &deployOptions{}}

	err := d.maybeWriteCustomSRKTemplate(esp, newTestSRKPublic())
	c.Check(err, check.ErrorMatches, "cannot open file: .*")
}

// readTemplate reads back the SRK template file written to esp and
// unmarshals it into a *tpm2.Public, failing the test on any error. The
// template is written as a TPM2B-style, length-prefixed blob (see
// (*imageDeployer).maybeWriteCustomSRKTemplate), so the outer wrapper must
// be stripped before the inner tpm2.Public can be decoded.
func readTemplate(c *check.C, esp string) *tpm2.Public {
	f, err := os.Open(filepath.Join(esp, "tpm2-srk.tmpl"))
	c.Assert(err, check.IsNil)
	defer f.Close()

	var raw []byte
	_, err = mu.UnmarshalFromReader(f, &raw)
	c.Assert(err, check.IsNil)

	var tmpl *tpm2.Public
	_, err = mu.UnmarshalFromBytes(raw, &tmpl)
	c.Assert(err, check.IsNil)
	return tmpl
}

func (s *deploySuite) TestReadSRKPublicAreaSuccess(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "srk.pub")

	pub := newTestSRKPublic()
	writeSRKPubFile(c, path, pub)

	d := &imageDeployer{opts: &deployOptions{SRKPub: path}}

	got, err := d.readSRKPublicArea()
	c.Assert(err, check.IsNil)
	c.Check(got.Type, check.Equals, pub.Type)
	c.Check(got.NameAlg, check.Equals, pub.NameAlg)
	c.Assert(got.Params, check.NotNil)
	c.Assert(got.Params.RSADetail, check.NotNil)
	c.Check(got.Params.RSADetail.KeyBits, check.Equals, pub.Params.RSADetail.KeyBits)
	c.Assert(got.Unique, check.NotNil)
	c.Check([]byte(got.Unique.RSA), check.DeepEquals, []byte(pub.Unique.RSA))
}

func (s *deploySuite) TestReadSRKPublicAreaFileNotFound(c *check.C) {
	dir := c.MkDir()
	d := &imageDeployer{opts: &deployOptions{SRKPub: filepath.Join(dir, "does-not-exist")}}

	_, err := d.readSRKPublicArea()
	c.Check(errors.Is(err, fs.ErrNotExist), check.Equals, true)
}

func (s *deploySuite) TestReadSRKPublicAreaCorruptOuterWrapper(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "srk.pub")
	// Claims a 0x64-byte payload but only provides two bytes of it.
	c.Assert(os.WriteFile(path, []byte{0x00, 0x64, 0x01, 0x02}, 0600), check.IsNil)

	d := &imageDeployer{opts: &deployOptions{SRKPub: path}}

	_, err := d.readSRKPublicArea()
	c.Check(err, check.ErrorMatches, "cannot unmarshal public area bytes: .*")
}

func (s *deploySuite) TestReadSRKPublicAreaCorruptInnerData(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "srk.pub")
	// Valid outer TPM2B wrapper, but the inner bytes don't decode to a
	// valid tpm2.Public.
	writeMuFile(c, path, []byte{0xde, 0xad, 0xbe, 0xef})

	d := &imageDeployer{opts: &deployOptions{SRKPub: path}}

	_, err := d.readSRKPublicArea()
	// Expected error is multi-line. Use (?s) to have ErrorMatches match against it.
	c.Check(err, check.ErrorMatches, "(?s)cannot unmarshal public area: .*")
}

func (s *deploySuite) TestNewEFIEnvironmentNoneConfigured(c *check.C) {
	d := &imageDeployer{opts: &deployOptions{}}

	env, err := d.newEFIEnvironment()
	c.Check(err, check.IsNil)
	c.Check(env, check.IsNil)
}

func (s *deploySuite) TestNewEFIEnvironmentAzDiskProfileFileNotFound(c *check.C) {
	dir := c.MkDir()
	d := &imageDeployer{opts: &deployOptions{AzDiskProfile: filepath.Join(dir, "does-not-exist")}}

	_, err := d.newEFIEnvironment()
	c.Check(err, check.ErrorMatches, "cannot open az disk profile resource: .*")
}

func (s *deploySuite) TestNewEFIEnvironmentAzDiskProfileInvalidJSON(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "disk.json")
	c.Assert(os.WriteFile(path, []byte("not json"), 0600), check.IsNil)

	d := &imageDeployer{opts: &deployOptions{AzDiskProfile: path}}

	_, err := d.newEFIEnvironment()
	c.Check(err, check.ErrorMatches, "cannot decode az disk profile resource: .*")
}

func (s *deploySuite) TestNewEFIEnvironmentAzDiskProfileUnexpectedType(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "disk.json")
	c.Assert(os.WriteFile(path, []byte(`{"type":"something/else"}`), 0600), check.IsNil)

	d := &imageDeployer{opts: &deployOptions{AzDiskProfile: path}}

	_, err := d.newEFIEnvironment()
	c.Check(err, check.ErrorMatches, "cannot create environment from az disk profile resource: .*")
}

func (s *deploySuite) TestNewEFIEnvironmentAzDiskProfileSuccess(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "disk.json")
	c.Assert(os.WriteFile(path, []byte(`{"type":"Microsoft.Compute/disks"}`), 0600), check.IsNil)

	d := &imageDeployer{opts: &deployOptions{AzDiskProfile: path}}

	env, err := d.newEFIEnvironment()
	c.Check(err, check.IsNil)
	c.Check(env, check.NotNil)
}

func (s *deploySuite) TestNewEFIEnvironmentUefiConfigFileNotFound(c *check.C) {
	dir := c.MkDir()
	d := &imageDeployer{opts: &deployOptions{UefiConfig: filepath.Join(dir, "does-not-exist")}}

	_, err := d.newEFIEnvironment()
	c.Check(err, check.ErrorMatches, "cannot open UEFI config: .*")
}

func (s *deploySuite) TestNewEFIEnvironmentUefiConfigInvalidJSON(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "uefi.json")
	c.Assert(os.WriteFile(path, []byte("not json"), 0600), check.IsNil)

	d := &imageDeployer{opts: &deployOptions{UefiConfig: path}}

	_, err := d.newEFIEnvironment()
	c.Check(err, check.ErrorMatches, "cannot decode UEFI config: .*")
}

func (s *deploySuite) TestNewEFIEnvironmentUefiConfigSuccess(c *check.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "uefi.json")
	c.Assert(os.WriteFile(path, []byte(`{}`), 0600), check.IsNil)

	d := &imageDeployer{opts: &deployOptions{UefiConfig: path}}

	env, err := d.newEFIEnvironment()
	c.Check(err, check.IsNil)
	c.Check(env, check.NotNil)
}

func (s *deploySuite) TestComputePCRProtectionProfileNoProfileSelected(c *check.C) {
	// None of the PCR profile options are enabled and no host environment
	// is provided, so the underlying library refuses to build an empty
	// profile. This exercises the error wrapping without requiring any
	// real EFI binaries, a TPM, or other hardware.
	d := &imageDeployer{opts: &deployOptions{}}

	_, err := d.computePCRProtectionProfile(c.MkDir(), nil)
	c.Check(err, check.ErrorMatches, "cannot add PCR profile: must specify a profile to add")
}

// TODO: add more testing of [imageDeployer.computePCRProtectionProfile]

func (s *deploySuite) TestReadKeyFromImageUnsupportedDevicePath(c *check.C) {
	d := &imageDeployer{}
	d.devPath = "/dev/unsupported0"
	d.partitions = gpt.Partitions{{Index: 1}}

	_, _, err := d.readKeyFromImage()
	c.Check(err, check.ErrorMatches, "unsupported device path: /dev/unsupported0.*")
}

func (s *deploySuite) TestReadKeyFromImageNoContainerFound(c *check.C) {
	d := &imageDeployer{}
	// /dev/sd-style paths resolve to e.g. "/dev/sdx1", which doesn't exist
	// on this system, so every partition is skipped.
	d.devPath = "/dev/sdxxxxx"
	d.partitions = gpt.Partitions{{Index: 1}, {Index: 2}}

	_, _, err := d.readKeyFromImage()
	c.Check(err, check.ErrorMatches, "no valid LUKS2 container found")
}

// TODO: add more testing of [imageDeployer.readKeyFromImage]
