// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"crypto"
	"io"
	"io/ioutil"
	"os"

	"github.com/canonical/go-efilib"
	"github.com/canonical/go-tpm2"
	"github.com/canonical/go-tpm2/linux"
	"github.com/canonical/tcglog-parser"
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

func (s *resealSuite) TestTrustedEfiImageOk(c *check.C) {
	s.writeFile(c, "/foo", 0, 43, 50)

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)
	c.Check(assets.TrustNewFromDir("/"), check.IsNil)

	context := new(pcrProfileComputeContext)
	img := newTrustedEFIImage(assets, context, "/foo")

	f, err := img.Open()
	c.Assert(err, check.IsNil)

	c.Check(f.Close(), check.IsNil)
	c.Check(context.nOpen, check.Equals, 0)
	c.Check(context.failedPaths, check.IsNil)
}

func (s *resealSuite) TestTrustedEfiImageBad(c *check.C) {
	s.writeFile(c, "/foo", 0, 43, 50)

	assets, err := ReadTrustedAssets()
	c.Assert(err, check.IsNil)

	context := new(pcrProfileComputeContext)
	img := newTrustedEFIImage(assets, context, "/foo")

	f, err := img.Open()
	c.Assert(err, check.IsNil)

	c.Check(f.Close(), check.IsNil)
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

	bm, err := NewBootManagerFromSystem()
	c.Assert(err, check.IsNil)
	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu", &bm)
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

	bm, err := NewBootManagerFromSystem()
	c.Assert(err, check.IsNil)
	km, err := NewKernelManager("/boot/efi", "/usr/lib/linux", "ubuntu", &bm)
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

// The TCG log writing code is borrowed from github.com:snapcore/secboot tools/make-efi-testdata/logs.go
// to avoid checking in a binary log

type event struct {
	PCRIndex  tcglog.PCRIndex
	EventType tcglog.EventType
	Data      tcglog.EventData
}

type eventData interface {
	Write(w io.Writer) error
}

type bytesData []byte

func (d bytesData) Write(w io.Writer) error {
	_, err := w.Write(d)
	return err
}

type logWriter struct {
	algs   []tpm2.HashAlgorithmId
	events []*tcglog.Event
}

func newCryptoAgileLogWriter() *logWriter {
	event := &tcglog.Event{
		PCRIndex:  0,
		EventType: tcglog.EventTypeNoAction,
		Digests:   tcglog.DigestMap{tpm2.HashAlgorithmSHA1: make(tcglog.Digest, tpm2.HashAlgorithmSHA1.Size())},
		Data: &tcglog.SpecIdEvent03{
			SpecVersionMajor: 2,
			UintnSize:        2,
			DigestSizes: []tcglog.EFISpecIdEventAlgorithmSize{
				{AlgorithmId: tpm2.HashAlgorithmSHA1, DigestSize: uint16(tpm2.HashAlgorithmSHA1.Size())},
				{AlgorithmId: tpm2.HashAlgorithmSHA256, DigestSize: uint16(tpm2.HashAlgorithmSHA256.Size())}}}}

	return &logWriter{
		algs:   []tpm2.HashAlgorithmId{tpm2.HashAlgorithmSHA1, tpm2.HashAlgorithmSHA256},
		events: []*tcglog.Event{event}}
}

func (w *logWriter) hashLogExtendEvent(data eventData, event *event) {
	ev := &tcglog.Event{
		PCRIndex:  event.PCRIndex,
		EventType: event.EventType,
		Digests:   make(tcglog.DigestMap),
		Data:      event.Data}

	for _, alg := range w.algs {
		h := alg.NewHash()
		if err := data.Write(h); err != nil {
			panic(err)
		}
		ev.Digests[alg] = h.Sum(nil)
	}

	w.events = append(w.events, ev)

}

func (s *resealSuite) writeMockTcglog(c *check.C) {
	w := newCryptoAgileLogWriter()

	{
		data := &tcglog.SeparatorEventData{Value: tcglog.SeparatorEventNormalValue}
		w.hashLogExtendEvent(data, &event{
			PCRIndex:  7,
			EventType: tcglog.EventTypeSeparator,
			Data:      data})
	}
	{
		data := tcglog.EFICallingEFIApplicationEvent
		w.hashLogExtendEvent(data, &event{
			PCRIndex:  4,
			EventType: tcglog.EventTypeEFIAction,
			Data:      data})
	}
	for _, pcr := range []tcglog.PCRIndex{0, 1, 2, 3, 4, 5, 6} {
		data := &tcglog.SeparatorEventData{Value: tcglog.SeparatorEventNormalValue}
		w.hashLogExtendEvent(data, &event{
			PCRIndex:  pcr,
			EventType: tcglog.EventTypeSeparator,
			Data:      data})
	}
	{
		pe := bytesData("mock shim PE")
		data := &tcglog.EFIImageLoadEvent{
			LocationInMemory: 0x6556c018,
			LengthInMemory:   955072,
			DevicePath: efi.DevicePath{
				&efi.ACPIDevicePathNode{
					HID: 0x0a0341d0,
					UID: 0x0},
				&efi.PCIDevicePathNode{
					Function: 0x0,
					Device:   0x1d},
				&efi.PCIDevicePathNode{
					Function: 0x0,
					Device:   0x0},
				&efi.NVMENamespaceDevicePathNode{
					NamespaceID:   0x1,
					NamespaceUUID: 0x0},
				&efi.HardDriveDevicePathNode{
					PartitionNumber: 1,
					PartitionStart:  0x800,
					PartitionSize:   0x100000,
					Signature:       efi.MakeGUID(0x66de947b, 0xfdb2, 0x4525, 0xb752, [...]uint8{0x30, 0xd6, 0x6b, 0xb2, 0xb9, 0x60}),
					MBRType:         efi.GPT},
				efi.FilePathDevicePathNode("\\EFI\\ubuntu\\shimx64.efi")}}
		w.hashLogExtendEvent(pe, &event{
			PCRIndex:  4,
			EventType: tcglog.EventTypeEFIBootServicesApplication,
			Data:      data})
	}
	{
		pe := bytesData("mock kernel PE")
		data := &tcglog.EFIImageLoadEvent{
			DevicePath: efi.DevicePath{efi.FilePathDevicePathNode("\\EFI\\ubuntu\\kernel.efi-1.0-1-generic")}}
		w.hashLogExtendEvent(pe, &event{
			PCRIndex:  4,
			EventType: tcglog.EventTypeEFIBootServicesApplication,
			Data:      data})
	}

	f, err := s.fs.OpenFile("/sys/kernel/security/tpm0/binary_bios_measurements", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	c.Assert(err, check.IsNil)
	defer f.Close()

	c.Check(tcglog.WriteLog(f, w.events), check.IsNil)
}

func (s *resealSuite) mockEfiComputePeImageDigest(fn func(alg crypto.Hash, r io.ReaderAt, sz int64) ([]byte, error)) (restore func()) {
	orig := efiComputePeImageDigest
	efiComputePeImageDigest = fn

	return func() {
		efiComputePeImageDigest = orig
	}
}

func (s *resealSuite) TestTrustCurrentBoot(c *check.C) {
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.writeMockTcglog(c)

	restore := s.mockEfiComputePeImageDigest(func(alg crypto.Hash, r io.ReaderAt, sz int64) ([]byte, error) {
		r2 := io.NewSectionReader(r, 0, sz)
		b, err := ioutil.ReadAll(r2)
		c.Check(err, check.IsNil)

		switch {
		case bytes.Equal(b, []byte("shim1")):
			return decodeHexString(c, "93c294bd9d372cf76e3cfd6f66a93fd2586aeb0406677ea0df104349b2ec093d"), nil
		case bytes.Equal(b, []byte("kernel1")):
			return decodeHexString(c, "54a5737f95928a359ba326bda6405a8e91fd06869cdb76f7f53aae83c1050308"), nil
		default:
			c.Fatal("invalid file")
		}
		return nil, nil
	})
	defer restore()

	assets := newTrustedAssets()

	c.Check(TrustCurrentBoot(assets, "/boot/efi"), check.IsNil)

	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "efbef08d5d3787d609ec6b55fabc36c7f212140b97a88606a39dc8f732368147"),
		decodeHexString(c, "7e8c4310bd1e228888917fb5f87920426dbecd64ea7d6c2256740f80e39dcf6f")})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte{
		decodeHexString(c, "efbef08d5d3787d609ec6b55fabc36c7f212140b97a88606a39dc8f732368147"),
		decodeHexString(c, "7e8c4310bd1e228888917fb5f87920426dbecd64ea7d6c2256740f80e39dcf6f")})
}

func (s *resealSuite) TestTrustCurrentBootRejectPeHashMismatch(c *check.C) {
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-1-generic", []byte("kernel1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.writeMockTcglog(c)

	restore := s.mockEfiComputePeImageDigest(func(alg crypto.Hash, r io.ReaderAt, sz int64) ([]byte, error) {
		r2 := io.NewSectionReader(r, 0, sz)
		b, err := ioutil.ReadAll(r2)
		c.Check(err, check.IsNil)

		switch {
		case bytes.Equal(b, []byte("shim1")):
			return decodeHexString(c, "93c294bd9d372cf76e3cfd6f66a93fd2586aeb0406677ea0df104349b2ec093f"), nil
		case bytes.Equal(b, []byte("kernel1")):
			return decodeHexString(c, "54a5737f95928a359ba326bda6405a8e91fd06869cdb76f7f53aae83c1050308"), nil
		default:
			c.Fatal("invalid file")
		}
		return nil, nil
	})
	defer restore()

	assets := newTrustedAssets()

	c.Check(TrustCurrentBoot(assets, "/boot/efi"), check.IsNil)

	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "7e8c4310bd1e228888917fb5f87920426dbecd64ea7d6c2256740f80e39dcf6f")})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte{
		decodeHexString(c, "7e8c4310bd1e228888917fb5f87920426dbecd64ea7d6c2256740f80e39dcf6f")})
}

func (s *resealSuite) TestTrustCurrentBootRejectMissing(c *check.C) {
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/shimx64.efi", []byte("shim1"), 0600), check.IsNil)
	c.Check(s.fs.WriteFile("/boot/efi/EFI/ubuntu/kernel.efi-1.0-2-generic", []byte("kernel2"), 0600), check.IsNil)

	s.writeMockTcglog(c)

	restore := s.mockEfiComputePeImageDigest(func(alg crypto.Hash, r io.ReaderAt, sz int64) ([]byte, error) {
		r2 := io.NewSectionReader(r, 0, sz)
		b, err := ioutil.ReadAll(r2)
		c.Check(err, check.IsNil)

		switch {
		case bytes.Equal(b, []byte("shim1")):
			return decodeHexString(c, "93c294bd9d372cf76e3cfd6f66a93fd2586aeb0406677ea0df104349b2ec093d"), nil
		case bytes.Equal(b, []byte("kernel1")):
			return decodeHexString(c, "54a5737f95928a359ba326bda6405a8e91fd06869cdb76f7f53aae83c1050308"), nil
		default:
			c.Fatal("invalid file")
		}
		return nil, nil
	})
	defer restore()

	assets := newTrustedAssets()

	c.Check(TrustCurrentBoot(assets, "/boot/efi"), check.IsNil)

	c.Check(assets.loaded.Hashes, check.DeepEquals, [][]byte{
		decodeHexString(c, "efbef08d5d3787d609ec6b55fabc36c7f212140b97a88606a39dc8f732368147")})
	c.Check(assets.newAssets, check.DeepEquals, [][]byte{
		decodeHexString(c, "efbef08d5d3787d609ec6b55fabc36c7f212140b97a88606a39dc8f732368147")})
}
