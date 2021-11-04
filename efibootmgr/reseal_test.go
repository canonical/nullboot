// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"errors"
	"io"
	"io/ioutil"
	"os"

	"github.com/canonical/go-efilib"
	"github.com/canonical/go-tpm2"
	"github.com/canonical/go-tpm2/linux"
	"github.com/snapcore/secboot"
	secboot_efi "github.com/snapcore/secboot/efi"
	secboot_tpm2 "github.com/snapcore/secboot/tpm2"

	"golang.org/x/sys/unix"

	"gopkg.in/check.v1"
)

type resealSuite struct {
	mapFsMixin
}

func (*resealSuite) mockSbefiAddBootManagerProfile(fn func(profile *secboot_tpm2.PCRProtectionProfile, params *secboot_efi.BootManagerProfileParams) error) (restore func()) {
	orig := sbefiAddBootManagerProfile
	sbefiAddBootManagerProfile = fn
	return func() {
		sbefiAddBootManagerProfile = orig
	}
}

func (*resealSuite) mockSbefiAddSecureBootPolicyProfile(fn func(profile *secboot_tpm2.PCRProtectionProfile, params *secboot_efi.SecureBootPolicyProfileParams) error) (restore func()) {
	orig := sbefiAddSecureBootPolicyProfile
	sbefiAddSecureBootPolicyProfile = fn
	return func() {
		sbefiAddSecureBootPolicyProfile = orig
	}
}

func (*resealSuite) mockSbGetAuxiliaryKeyFromKernel(fn func(prefix, devicePath string, remove bool) (secboot.AuxiliaryKey, error)) (restore func()) {
	orig := sbGetAuxiliaryKeyFromKernel
	sbGetAuxiliaryKeyFromKernel = fn
	return func() {
		sbGetAuxiliaryKeyFromKernel = orig
	}
}

func (*resealSuite) mockSbtpmConnectToDefaultTPM(fn func() (*secboot_tpm2.Connection, error)) (restore func()) {
	orig := sbtpmConnectToDefaultTPM
	sbtpmConnectToDefaultTPM = fn
	return func() {
		sbtpmConnectToDefaultTPM = orig
	}
}

func (*resealSuite) mockSbtpmReadSealedKeyObjectFromFile(fn func(path string) (*secboot_tpm2.SealedKeyObject, error)) (restore func()) {
	orig := sbtpmReadSealedKeyObjectFromFile
	sbtpmReadSealedKeyObjectFromFile = fn
	return func() {
		sbtpmReadSealedKeyObjectFromFile = orig
	}
}

func (*resealSuite) mockSbtpmSealedKeyObjectUpdatePCRProtectionPolicy(fn func(k *secboot_tpm2.SealedKeyObject, tpm *secboot_tpm2.Connection, authKey secboot_tpm2.PolicyAuthKey, profile *secboot_tpm2.PCRProtectionProfile) error) (restore func()) {
	orig := sbtpmSealedKeyObjectUpdatePCRProtectionPolicy
	sbtpmSealedKeyObjectUpdatePCRProtectionPolicy = fn
	return func() {
		sbtpmSealedKeyObjectUpdatePCRProtectionPolicy = orig
	}
}

func (*resealSuite) mockSbtpmSealedKeyObjectWriteAtomic(fn func(k *secboot_tpm2.SealedKeyObject, w secboot.KeyDataWriter) error) (restore func()) {
	orig := sbtpmSealedKeyObjectWriteAtomic
	sbtpmSealedKeyObjectWriteAtomic = fn
	return func() {
		sbtpmSealedKeyObjectWriteAtomic = orig
	}
}

func (*resealSuite) mockUnixKeyctlInt(fn func(cmd, arg2, arg3, arg4, arg5 int) (int, error)) (restore func()) {
	orig := unixKeyctlInt
	unixKeyctlInt = fn
	return func() {
		unixKeyctlInt = orig
	}
}

func (*resealSuite) mockEfiVars(vars map[efi.VariableDescriptor]mockEFIVariable) (restore func()) {
	orig := appEFIVars
	appEFIVars = &MockEFIVariables{vars}
	return func() {
		appEFIVars = orig
	}
}

func (*resealSuite) mockEfiArch(arch string) (restore func()) {
	orig := appArchitecture
	appArchitecture = arch
	return func() {
		appArchitecture = orig
	}
}

var _ = check.Suite(&resealSuite{})

type testEfiImageFileReadBlock struct {
	off int64
	sz  int64
	n   int64
}

func (s *resealSuite) testEfiImageFile(c *check.C, path string, blocks []testEfiImageFileReadBlock) {
	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	c.Check(assets.TrustNewFromDir("/"), check.IsNil)

	context := &pcrProfileComputeContext{assets: assets, nOpen: 1}

	f, err := appFs.Open(path)
	c.Assert(err, check.IsNil)
	defer f.Close()

	ef, err := newEfiImageFile(context, f)
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
			n, err := ef.ReadAt(data[i:i+block.sz], i+block.off)
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) || int64(n) < block.sz {
				break
			}
			c.Check(err, check.IsNil)
			i += int64(n)
		}

		c.Check(data, check.DeepEquals, expected)
	}

	c.Check(ef.Close(), check.IsNil)
	c.Check(context.nOpen, check.Equals, 0)
	c.Check(context.failedPaths, check.DeepEquals, []string(nil))
}

func (s *resealSuite) TestEfiImageFileReadFullSmallReads(c *check.C) {
	s.writeFile(c, "/foo", 0, 199, 3500)
	s.testEfiImageFile(c, "/foo", []testEfiImageFileReadBlock{
		{off: 0, sz: 10, n: 69650},
	})
}

func (s *resealSuite) TestEfiImageFileReadFullLargeReads(c *check.C) {
	s.writeFile(c, "/foo", 0, 199, 3500)
	s.testEfiImageFile(c, "/foo", []testEfiImageFileReadBlock{
		{off: 0, sz: 69650, n: 10},
	})
}

func (s *resealSuite) TestEfiImageFileReadSparse(c *check.C) {
	s.writeFile(c, "/foo", 0, 199, 3500)
	s.testEfiImageFile(c, "/foo", []testEfiImageFileReadBlock{
		{off: 500, sz: 10, n: 100},
		{off: 20000, sz: 500, n: 20},
	})
}

func (s *resealSuite) TestEfiImageFileReadUntrusted(c *check.C) {
	s.writeFile(c, "/foo", 0, 43, 4000)

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	context := &pcrProfileComputeContext{assets: assets, nOpen: 1}

	f, err := appFs.Open("/foo")
	c.Assert(err, check.IsNil)
	defer f.Close()

	ef, err := newEfiImageFile(context, f)
	c.Assert(err, check.IsNil)

	for i := int64(0); ; {
		var data [30]byte
		n, err := ef.ReadAt(data[:], i)
		if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) || n < 30 {
			break
		}
		c.Check(err, check.IsNil)
		i += int64(n)
	}

	c.Check(ef.Close(), check.IsNil)
	c.Check(context.nOpen, check.Equals, 0)
	c.Check(context.failedPaths, check.DeepEquals, []string{"/foo"})
}

type testResealKeyData struct {
	arch         string
	auxiliaryKey []byte
	devicePaths  []string
	shims        [][]byte
	kernels      [][]byte
}

func (s *resealSuite) testResealKey(c *check.C, data *testResealKeyData) {
	var (
		expectedSko                         *secboot_tpm2.SealedKeyObject = nil
		expectedTpm                         *secboot_tpm2.Connection      = nil
		userKeyringLinkedFromProcessKeyring                               = false
	)

	restore := s.mockEfiArch(data.arch)
	defer restore()

	restore = s.mockSbefiAddBootManagerProfile(func(profile *secboot_tpm2.PCRProtectionProfile, params *secboot_efi.BootManagerProfileParams) error {
		c.Assert(profile, check.NotNil)
		c.Check(params.PCRAlgorithm, check.Equals, tpm2.HashAlgorithmSHA256)

		c.Assert(params.LoadSequences, check.HasLen, len(data.shims))
		for i, e := range params.LoadSequences {
			f, err := e.Image.Open()
			c.Assert(err, check.IsNil)

			r := io.NewSectionReader(f, 0, 1<<63-1)
			b, err := ioutil.ReadAll(r)
			c.Check(err, check.IsNil)
			f.Close()

			c.Check(b, check.DeepEquals, data.shims[i])

			c.Assert(e.Next, check.HasLen, len(data.kernels))
			for i, e := range e.Next {
				f, err := e.Image.Open()
				c.Assert(err, check.IsNil)

				r := io.NewSectionReader(f, 0, 1<<63-1)
				b, err := ioutil.ReadAll(r)
				c.Check(err, check.IsNil)
				f.Close()

				c.Check(b, check.DeepEquals, data.kernels[i])
			}
		}

		profile.AddPCRValue(tpm2.HashAlgorithmSHA256, 4, make([]byte, 32))
		return nil
	})
	defer restore()

	restore = s.mockSbefiAddSecureBootPolicyProfile(func(profile *secboot_tpm2.PCRProtectionProfile, params *secboot_efi.SecureBootPolicyProfileParams) error {
		c.Assert(profile, check.NotNil)
		c.Check(params.PCRAlgorithm, check.Equals, tpm2.HashAlgorithmSHA256)

		c.Assert(params.LoadSequences, check.HasLen, len(data.shims))
		for i, e := range params.LoadSequences {
			c.Check(e.Source, check.Equals, secboot_efi.Firmware)

			f, err := e.Image.Open()
			c.Assert(err, check.IsNil)

			r := io.NewSectionReader(f, 0, 1<<63-1)
			b, err := ioutil.ReadAll(r)
			c.Check(err, check.IsNil)
			f.Close()

			c.Check(b, check.DeepEquals, data.shims[i])

			c.Assert(e.Next, check.HasLen, len(data.kernels))
			for i, e := range e.Next {
				c.Check(e.Source, check.Equals, secboot_efi.Shim)

				f, err := e.Image.Open()
				c.Assert(err, check.IsNil)

				r := io.NewSectionReader(f, 0, 1<<63-1)
				b, err := ioutil.ReadAll(r)
				c.Check(err, check.IsNil)
				f.Close()

				c.Check(b, check.DeepEquals, data.kernels[i])
			}
		}

		profile.AddPCRValue(tpm2.HashAlgorithmSHA256, 7, make([]byte, 32))
		return nil
	})
	defer restore()

	n := 0
	restore = s.mockSbGetAuxiliaryKeyFromKernel(func(prefix, devicePath string, remove bool) (secboot.AuxiliaryKey, error) {
		c.Check(prefix, check.Equals, "ubuntu-fde")
		c.Check(devicePath, check.Equals, data.devicePaths[n])
		c.Check(remove, check.Equals, false)

		c.Check(userKeyringLinkedFromProcessKeyring, check.Equals, true)

		n++
		if n < len(data.devicePaths) {
			return nil, secboot.ErrKernelKeyNotFound
		}

		return data.auxiliaryKey, nil
	})
	defer restore()

	restore = s.mockSbtpmConnectToDefaultTPM(func() (*secboot_tpm2.Connection, error) {
		c.Check(expectedTpm, check.IsNil)

		tcti, err := linux.OpenDevice("/dev/null")
		c.Assert(err, check.IsNil)

		expectedTpm = &secboot_tpm2.Connection{TPMContext: tpm2.NewTPMContext(tcti)}
		return expectedTpm, nil
	})
	defer restore()

	restore = s.mockSbtpmReadSealedKeyObjectFromFile(func(path string) (*secboot_tpm2.SealedKeyObject, error) {
		c.Check(expectedSko, check.IsNil)

		c.Check(path, check.Equals, "/boot/efi/device/fde/cloudimg-rootfs.sealed-key")

		expectedSko = &secboot_tpm2.SealedKeyObject{}
		return expectedSko, nil
	})
	defer restore()

	restore = s.mockSbtpmSealedKeyObjectUpdatePCRProtectionPolicy(func(k *secboot_tpm2.SealedKeyObject, tpm *secboot_tpm2.Connection, authKey secboot_tpm2.PolicyAuthKey, profile *secboot_tpm2.PCRProtectionProfile) error {
		c.Check(k, check.Equals, expectedSko)
		c.Check(tpm, check.Equals, expectedTpm)
		c.Check(authKey, check.DeepEquals, secboot_tpm2.PolicyAuthKey(data.auxiliaryKey))
		c.Assert(profile, check.NotNil)

		pcrs, _, err := profile.ComputePCRDigests(nil, tpm2.HashAlgorithmSHA256)
		c.Check(err, check.IsNil)
		c.Check(pcrs.Equal(tpm2.PCRSelectionList{{Hash: tpm2.HashAlgorithmSHA256, Select: []int{4, 7, 12}}}), check.Equals, true)
		return nil
	})
	defer restore()

	restore = s.mockSbtpmSealedKeyObjectWriteAtomic(func(k *secboot_tpm2.SealedKeyObject, w secboot.KeyDataWriter) error {
		c.Check(k, check.Equals, expectedSko)
		fw, ok := w.(*secboot_tpm2.FileSealedKeyObjectWriter)
		c.Check(ok, check.Equals, true)
		c.Check(fw, check.NotNil)
		return nil
	})
	defer restore()

	restore = s.mockUnixKeyctlInt(func(cmd, arg2, arg3, arg4, arg5 int) (int, error) {
		if cmd == unix.KEYCTL_LINK && arg2 == -4 && arg3 == -2 && arg4 == 0 && arg5 == 0 {
			userKeyringLinkedFromProcessKeyring = true
		}
		return 0, nil
	})
	defer restore()

	restore = s.mockEfiVars(map[efi.VariableDescriptor]mockEFIVariable{{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{1, 0, 2, 0, 3, 0}, 123}})
	defer restore()

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	c.Check(assets.TrustNewFromDir("/boot/efi/EFI/ubuntu"), check.IsNil)

	c.Check(assets.TrustNewFromDir("/usr/lib/nullboot/shim"), check.IsNil)
	c.Check(assets.TrustNewFromDir("/usr/lib/linux"), check.IsNil)

	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu")
	c.Assert(err, check.IsNil)

	c.Check(ResealKey(assets, km, "/boot/efi", "/usr/lib/nullboot/shim", "ubuntu"), check.IsNil)
}

func (s *resealSuite) TestResealKeyNoFDE(c *check.C) {
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel-efi.1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{})
}

func (s *resealSuite) TestResealKeyBeforeNewKernel(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "x64",
		auxiliaryKey: []byte{1, 2, 3, 4, 5, 6},
		devicePaths:  []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyAfterNewKernel(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "x64",
		auxiliaryKey: []byte{1, 2, 3, 4, 5, 6},
		devicePaths:  []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel1"),
			[]byte("kernel2"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyBeforeNewShim(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "x64",
		auxiliaryKey: []byte{1, 2, 3, 4, 5, 6},
		devicePaths:  []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyAfterNewShim(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "x64",
		auxiliaryKey: []byte{1, 2, 3, 4, 5, 6},
		devicePaths:  []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim2"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyBeforeRemoveKernel(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "x64",
		auxiliaryKey: []byte{1, 2, 3, 4, 5, 6},
		devicePaths:  []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel2"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyAfterRemoveKernel(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "x64",
		auxiliaryKey: []byte{1, 2, 3, 4, 5, 6},
		devicePaths:  []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel2"),
		},
	})
}

func (s *resealSuite) TestResealKeyDifferentAuxiliaryKey(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "x64",
		auxiliaryKey: []byte{5, 6, 7, 8, 9},
		devicePaths:  []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyDifferentBlockDevice(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/vda14", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/vda14", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "x64",
		auxiliaryKey: []byte{1, 2, 3, 4, 5, 6},
		devicePaths:  []string{"/dev/vda14"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyDifferentArch(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimaa64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimaa64.efi.signed", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "aa64",
		auxiliaryKey: []byte{1, 2, 3, 4, 5, 6},
		devicePaths:  []string{"/dev/sda1"},
		shims: [][]byte{
			[]byte("shim1"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel2"),
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

func (s *resealSuite) TestResealKeyGetAuxiliaryKeyFromKernelBug(c *check.C) {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	c.Check(s.fs.WriteFile("/dev/sda15", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")
	s.symlink(c, "/dev/sda1", "/dev/disk/by-partuuid/94725587-885d-4bde-bc61-078e0010057d")
	s.symlink(c, "/dev/sda15", "/dev/disk/by-partuuid/848b8304-0f20-42e9-9806-b447ce344d85")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	s.testResealKey(c, &testResealKeyData{
		arch:         "x64",
		auxiliaryKey: []byte{1, 2, 3, 4, 5, 6},
		devicePaths:  []string{"/dev/sda1", "/dev/disk/by-partuuid/94725587-885d-4bde-bc61-078e0010057d"},
		shims: [][]byte{
			[]byte("shim2"),
			[]byte("shim1"),
		},
		kernels: [][]byte{
			[]byte("kernel1"),
			[]byte("kernel1"),
		},
	})
}

type testResealKeyUnhappyData struct {
	noAuxKey        bool
	fileLeak        bool
	untrustedAssets bool
	noTpm           bool
}

func (s *resealSuite) testResealKeyUnhappy(c *check.C, data *testResealKeyUnhappyData) error {
	c.Check(s.fs.WriteFile("/dev/sda1", nil, os.ModeDevice|0660), check.IsNil)
	s.symlink(c, "/dev/sda1", "/dev/disk/by-label/cloudimg-rootfs-enc")

	c.Check(s.fs.WriteFile("/boot/efi/device/fde/cloudimg-rootfs.sealed-key", []byte("key data"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/nullboot/shim/shimx64.efi.signed", []byte("shim2"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/usr/lib/linux/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)

	restore := s.mockEfiArch("x64")
	defer restore()

	restore = s.mockSbefiAddBootManagerProfile(func(profile *secboot_tpm2.PCRProtectionProfile, params *secboot_efi.BootManagerProfileParams) error {
		if data.fileLeak {
			params.LoadSequences[0].Image.Open()
		}
		for _, e := range params.LoadSequences {
			f, err := e.Image.Open()
			c.Assert(err, check.IsNil)
			f.Close()

			for _, e := range e.Next {
				f, err := e.Image.Open()
				c.Assert(err, check.IsNil)
				f.Close()
			}
		}
		return nil
	})
	defer restore()

	restore = s.mockSbefiAddSecureBootPolicyProfile(func(profile *secboot_tpm2.PCRProtectionProfile, params *secboot_efi.SecureBootPolicyProfileParams) error {
		for _, e := range params.LoadSequences {
			f, err := e.Image.Open()
			c.Assert(err, check.IsNil)
			f.Close()

			for _, e := range e.Next {
				f, err := e.Image.Open()
				c.Assert(err, check.IsNil)
				f.Close()
			}
		}
		return nil
	})
	defer restore()

	restore = s.mockSbGetAuxiliaryKeyFromKernel(func(prefix, devicePath string, remove bool) (secboot.AuxiliaryKey, error) {
		if data.noAuxKey {
			return nil, secboot.ErrKernelKeyNotFound
		}
		return nil, nil
	})
	defer restore()

	restore = s.mockSbtpmConnectToDefaultTPM(func() (*secboot_tpm2.Connection, error) {
		if data.noTpm {
			return nil, secboot_tpm2.ErrNoTPM2Device
		}
		tcti, err := linux.OpenDevice("/dev/null")
		c.Assert(err, check.IsNil)

		return &secboot_tpm2.Connection{TPMContext: tpm2.NewTPMContext(tcti)}, nil
	})
	defer restore()

	restore = s.mockSbtpmReadSealedKeyObjectFromFile(func(path string) (*secboot_tpm2.SealedKeyObject, error) {
		return &secboot_tpm2.SealedKeyObject{}, nil
	})
	defer restore()

	restore = s.mockSbtpmSealedKeyObjectUpdatePCRProtectionPolicy(func(k *secboot_tpm2.SealedKeyObject, tpm *secboot_tpm2.Connection, authKey secboot_tpm2.PolicyAuthKey, profile *secboot_tpm2.PCRProtectionProfile) error {
		return nil
	})
	defer restore()

	restore = s.mockSbtpmSealedKeyObjectWriteAtomic(func(k *secboot_tpm2.SealedKeyObject, w secboot.KeyDataWriter) error {
		return nil
	})
	defer restore()

	restore = s.mockUnixKeyctlInt(func(cmd, arg2, arg3, arg4, arg5 int) (int, error) {
		return 0, nil
	})
	defer restore()

	restore = s.mockEfiVars(map[efi.VariableDescriptor]mockEFIVariable{{GUID: efi.GlobalVariable, Name: "BootOrder"}: {[]byte{1, 0, 2, 0, 3, 0}, 123}})
	defer restore()

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	if !data.untrustedAssets {
		c.Check(assets.TrustNewFromDir("/boot/efi/EFI/ubuntu"), check.IsNil)
	}
	c.Check(assets.TrustNewFromDir("/usr/lib/nullboot/shim"), check.IsNil)
	c.Check(assets.TrustNewFromDir("/usr/lib/linux"), check.IsNil)

	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu")
	c.Assert(err, check.IsNil)

	return ResealKey(assets, km, "/boot/efi", "/usr/lib/nullboot/shim", "ubuntu")
}

func (s *resealSuite) TestResealKeyUnhappyNoAuxiliaryKey(c *check.C) {
	err := s.testResealKeyUnhappy(c, &testResealKeyUnhappyData{
		noAuxKey: true,
	})
	c.Check(err, check.ErrorMatches, "cannot obtain auth key from kernel: cannot read key from kernel: cannot find key in kernel keyring")
}

func (s *resealSuite) TestResealKeyUnhappyFileLeak(c *check.C) {
	err := s.testResealKeyUnhappy(c, &testResealKeyUnhappyData{
		fileLeak: true,
	})
	c.Check(err, check.ErrorMatches, "leaked open files from computing PCR profile")
}

func (s *resealSuite) TestResealKeyUnhappyUntrustedAssets(c *check.C) {
	err := s.testResealKeyUnhappy(c, &testResealKeyUnhappyData{
		untrustedAssets: true,
	})
	c.Check(err, check.ErrorMatches, "some assets failed an integrity check: \\[/boot/efi/EFI/ubuntu/shimx64.efi /boot/efi/EFI/ubuntu/shimx64.efi\\]")
}

func (s *resealSuite) TestResealKeyUnhappyNoTPM(c *check.C) {
	err := s.testResealKeyUnhappy(c, &testResealKeyUnhappyData{
		noTpm: true,
	})
	c.Check(err, check.ErrorMatches, "no TPM2 device is available")
}
