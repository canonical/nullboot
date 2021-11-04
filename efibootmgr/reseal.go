// This file is part of nullboot
// Copyright 2021 Canonical Ltd.
// SPDX-License-Identifier: GPL-3.0-only

package efibootmgr

import (
	"bytes"
	"crypto"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"github.com/canonical/go-tpm2"
	"github.com/snapcore/secboot"
	secboot_efi "github.com/snapcore/secboot/efi"
	secboot_tpm2 "github.com/snapcore/secboot/tpm2"

	"golang.org/x/sys/unix"
)

const (
	keyFilePath   = "device/fde/cloudimg-rootfs.sealed-key"
	keyringPrefix = "ubuntu-fde"
	rootfsLabel   = "cloudimg-rootfs-enc"
)

var (
	sbefiAddBootManagerProfile                    = secboot_efi.AddBootManagerProfile
	sbefiAddSecureBootPolicyProfile               = secboot_efi.AddSecureBootPolicyProfile
	sbGetAuxiliaryKeyFromKernel                   = secboot.GetAuxiliaryKeyFromKernel
	sbtpmConnectToDefaultTPM                      = secboot_tpm2.ConnectToDefaultTPM
	sbtpmReadSealedKeyObjectFromFile              = secboot_tpm2.ReadSealedKeyObjectFromFile
	sbtpmSealedKeyObjectUpdatePCRProtectionPolicy = (*secboot_tpm2.SealedKeyObject).UpdatePCRProtectionPolicy
	sbtpmSealedKeyObjectWriteAtomic               = (*secboot_tpm2.SealedKeyObject).WriteAtomic

	unixKeyctlInt = unix.KeyctlInt
)

type pcrProfileComputeContext struct {
	assets      *TrustedAssets
	nOpen       int
	failedPaths []string
}

// efiImageFile wraps a file handle and is used by the PCR profile generation
// to access boot assets, and to check that the boot assets included in the PCR
// profile are trusted.
//
// During read operations, leaf hashes are computed on a block-by-block basis.
// If a block's hash hasn't previously been computed, then it is recorded. If it
// has previously been computed, then the hash is compared against the previously
// recoreded one.
//
// During close, any previously unread blocks are read in order to compute their
// leaf hashes and construct a hash tree in order to generate a root hash, which
// is then compared to the list of trusted asset hashes via TrustedAssets. If the
// root hash is not included in the list of trusted hashes, then the generated PCR
// profile is rejected by signalling the failure via pcrProfileComputeContext.
//
// This verification is done without having to read and keep entire PE images in
// memory, which would be the case if we only stored a flat-file hash. See the
// documentation for TrustedAssets to see the tradeoff of not storing the hash
// tree though.
type efiImageFile struct {
	context          *pcrProfileComputeContext
	leafHashes       [][]byte
	cachedBlockIndex int64
	cachedBlock      []byte
	file             File
	sz               int64
}

func newEfiImageFile(context *pcrProfileComputeContext, f File) (*efiImageFile, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	return &efiImageFile{
		context:          context,
		leafHashes:       make([][]byte, (info.Size()+(hashBlockSize-1))/hashBlockSize),
		cachedBlockIndex: -1,
		file:             f,
		sz:               info.Size()}, nil
}

func (f *efiImageFile) readAndCacheBlock(i int64) error {
	if i == f.cachedBlockIndex {
		// Reading from the cached block
		return nil
	}

	if i >= int64(len(f.leafHashes)) {
		// Huh, out of range
		return io.EOF
	}

	// Read the whole block
	r := io.NewSectionReader(f.file, i*hashBlockSize, hashBlockSize)

	var block [hashBlockSize]byte
	n, err := io.ReadFull(r, block[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		// Handle io.ErrUnexpectedEOF later.
		return err
	}

	// Cache this block to speed up small reads
	f.cachedBlockIndex = i
	f.cachedBlock = block[:n]

	// Hash the block
	h := f.context.assets.alg().New()
	h.Write(block[:])

	if len(f.leafHashes[i]) == 0 {
		// This is the first time we read this block.
		f.leafHashes[i] = h.Sum(nil)
	} else if !bytes.Equal(h.Sum(nil), f.leafHashes[i]) {
		// We've read this block before, and it has changed.
		return fmt.Errorf("hash check fail for block %d", i)
	}

	return err
}

func (f *efiImageFile) ReadAt(p []byte, off int64) (n int, err error) {
	// Calculate the starting block and number of blocks.
	start := ((off + hashBlockSize) / hashBlockSize) - 1
	end := ((off + int64(len(p)) + hashBlockSize) / hashBlockSize)
	num := end - start

	// Read and hash each block.
	for i := start; i < start+num; i++ {
		if err := f.readAndCacheBlock(i); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}

		data := f.cachedBlock
		if n == 0 {
			off0 := off - (start * hashBlockSize)
			data = data[off0:]
		}
		sz := len(p) - n
		if sz < len(data) {
			data = data[:sz]
		}

		copy(p[n:], data)
		n += len(data)

		if err != nil {
			break
		}
	}

	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (f *efiImageFile) Close() error {
	h := f.context.assets.alg().New()

	// Loop over missing leaf hashes.
	for i, d := range f.leafHashes {
		if len(d) > 0 {
			continue
		}

		// Hash missing block.
		r := io.NewSectionReader(f.file, int64(i*hashBlockSize), hashBlockSize)

		var block [hashBlockSize]byte
		_, err := io.ReadFull(r, block[:])
		if err == io.EOF {
			break
		}
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}

		h.Reset()
		h.Write(block[:])
		f.leafHashes[i] = h.Sum(nil)

		if err != nil {
			break
		}
	}

	// Compute root hash and make sure we trust this file.
	if !f.context.assets.checkLeafHashes(f.leafHashes) {
		f.context.failedPaths = append(f.context.failedPaths, f.file.Name())
	}

	f.context.nOpen--
	return f.file.Close()
}

func (f *efiImageFile) Size() int64 {
	return f.sz
}

type trustedEFIImage struct {
	context *pcrProfileComputeContext
	path    string
}

func (i *trustedEFIImage) String() string {
	return i.path
}

func (i *trustedEFIImage) Open() (file interface {
	io.ReaderAt
	io.Closer
	Size() int64
}, err error) {
	f, err := appFs.Open(i.path)
	if err != nil {
		return nil, err
	}
	i.context.nOpen++

	defer func() {
		if err != nil {
			f.Close()
			i.context.nOpen--
		}
	}()

	return newEfiImageFile(i.context, f)
}

func newTrustedEFIImage(context *pcrProfileComputeContext, path string) *trustedEFIImage {
	return &trustedEFIImage{context, path}
}

func resolveLink(path string) (string, error) {
	path = filepath.Clean(path)

	for {
		tgtPath, err := appFs.Readlink(path)

		if errors.Is(err, syscall.EINVAL) {
			return path, nil
		}

		if err != nil {
			return "", err
		}

		if !filepath.IsAbs(tgtPath) {
			tgtPath = filepath.Clean(filepath.Join(filepath.Dir(path), tgtPath))
		}

		path = tgtPath
	}
}

func getPolicyAuthKeyFromKernel() (secboot_tpm2.PolicyAuthKey, error) {
	devPath, err := resolveLink(filepath.Join("/dev/disk/by-label", rootfsLabel))
	if err != nil {
		return nil, fmt.Errorf("cannot resolve devive symlink: %w", err)
	}

	// By default, system services get their own session keyring that doesn't have
	// the user keyring linked to it. This means that attempting to read a key from
	// the user keyring will fail if the key only permits possessor read. Link the
	// user keyring into our process keyring so that we can read such keys from the
	// user keyring.
	if _, err := unixKeyctlInt(unix.KEYCTL_LINK, -4, -2, 0, 0); err != nil {
		return nil, fmt.Errorf("cannot link user keyring into process keyring: %w", err)
	}

	key, err := sbGetAuxiliaryKeyFromKernel(keyringPrefix, devPath, false)
	if err != nil {
		if err == secboot.ErrKernelKeyNotFound {
			// Work around a secboot bug
			ents, err2 := appFs.ReadDir("/dev/disk/by-partuuid")
			if err2 == nil {
				for _, ent := range ents {
					path := filepath.Join("/dev/disk/by-partuuid", ent.Name())
					devPath2, err2 := resolveLink(path)
					if err2 != nil {
						continue
					}

					if devPath2 == devPath {
						key, err = sbGetAuxiliaryKeyFromKernel(keyringPrefix, path, false)
						break
					}
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read key from kernel: %w", err)
		}
	}

	return secboot_tpm2.PolicyAuthKey(key), nil
}

func computePCRProtectionProfile(loadChains []*secboot_efi.ImageLoadEvent) (*secboot_tpm2.PCRProtectionProfile, error) {
	profile := secboot_tpm2.NewPCRProtectionProfile()

	pcr4Params := secboot_efi.BootManagerProfileParams{
		PCRAlgorithm:  tpm2.HashAlgorithmSHA256,
		LoadSequences: loadChains}
	if err := sbefiAddBootManagerProfile(profile, &pcr4Params); err != nil {
		return nil, fmt.Errorf("cannot add EFI boot manager profile: %w", err)
	}

	pcr7Params := secboot_efi.SecureBootPolicyProfileParams{
		PCRAlgorithm:  tpm2.HashAlgorithmSHA256,
		LoadSequences: loadChains}
	if err := sbefiAddSecureBootPolicyProfile(profile, &pcr7Params); err != nil {
		return nil, fmt.Errorf("cannot add EFI secure boot policy profile: %w", err)
	}

	profile.AddPCRValue(tpm2.HashAlgorithmSHA256, 12, make([]byte, tpm2.HashAlgorithmSHA256.Size()))

	// snap-bootstrap measures an epoch
	h := crypto.SHA256.New()
	binary.Write(h, binary.LittleEndian, uint32(0))
	profile.ExtendPCR(tpm2.HashAlgorithmSHA256, 12, h.Sum(nil))

	// XXX: The kernel EFI stub has a compiled-in commandline which isn't measured.

	log.Println("Computed PCR profile:", profile)
	pcrValues, err := profile.ComputePCRValues(nil)
	if err != nil {
		return nil, fmt.Errorf("cannot compute PCR values: %w", err)
	}
	log.Println("Computed PCR values:")
	for i, values := range pcrValues {
		log.Printf(" branch %d:\n", i)
		for alg := range values {
			for pcr := range values[alg] {
				log.Printf("  PCR%d,%v: %x\n", pcr, alg, values[alg][pcr])
			}
		}
	}
	pcrs, digests, err := profile.ComputePCRDigests(nil, tpm2.HashAlgorithmSHA256)
	if err != nil {
		return nil, fmt.Errorf("cannot compute PCR digests: %w", err)
	}
	log.Println("PCR selection:", pcrs)
	log.Println("Computed PCR digests:")
	for _, digest := range digests {
		log.Printf(" %x\n", digest)
	}

	return profile, nil
}

// ResealKey updates the PCR profile for the disk encryption key to incorporate
// the boot assets installed directly by the package manager and those assets
// copied by this package to the ESP.
func ResealKey(assets *TrustedAssets, km *KernelManager, esp, shimSource, vendor string) error {
	_, err := appFs.Stat(filepath.Join(esp, keyFilePath))
	if os.IsNotExist(err) {
		// Assume that this file being missing means there is nothing to do.
		return nil
	}

	context := &pcrProfileComputeContext{assets: assets}

	shimBase := "shim" + GetEfiArchitecture() + ".efi"

	var roots []*secboot_efi.ImageLoadEvent

	for _, path := range []string{
		filepath.Join(shimSource, shimBase+".signed"),
		filepath.Join(esp, "EFI", vendor, shimBase)} {
		_, err := appFs.Stat(path)
		if os.IsNotExist(err) {
			continue
		}

		roots = append(roots, &secboot_efi.ImageLoadEvent{
			Source: secboot_efi.Firmware,
			Image:  newTrustedEFIImage(context, path)})
	}

	var kernels []*secboot_efi.ImageLoadEvent

	for _, x := range []struct {
		dir   string
		files []string
	}{
		{
			dir:   km.sourceDir,
			files: km.sourceKernels,
		},
		{
			dir:   km.targetDir,
			files: km.targetKernels,
		},
	} {
		for _, n := range x.files {
			path := filepath.Join(x.dir, n)

			kernels = append(kernels, &secboot_efi.ImageLoadEvent{
				Source: secboot_efi.Shim,
				Image:  newTrustedEFIImage(context, path)})
		}
	}

	for _, root := range roots {
		root.Next = kernels
	}

	authKey, err := getPolicyAuthKeyFromKernel()
	if err != nil {
		return fmt.Errorf("cannot obtain auth key from kernel: %w", err)
	}

	pcrProfile, err := computePCRProtectionProfile(roots)
	if err != nil {
		return fmt.Errorf("cannot compute PCR profile: %w", err)
	}

	if context.nOpen != 0 {
		return errors.New("leaked open files from computing PCR profile")
	}

	if len(context.failedPaths) > 0 {
		return fmt.Errorf("some assets failed an integrity check: %v", context.failedPaths)
	}

	k, err := sbtpmReadSealedKeyObjectFromFile(filepath.Join(esp, keyFilePath))
	if err != nil {
		return fmt.Errorf("cannot read sealed key file: %w", err)
	}

	// XXX: Connection is required because we do integrity checks
	// on the key data. Should probably switch to using the /dev/tpmrm0
	// device here.
	tpm, err := sbtpmConnectToDefaultTPM()
	if err != nil {
		return err
	}
	defer tpm.Close()

	if err := sbtpmSealedKeyObjectUpdatePCRProtectionPolicy(k, tpm, authKey, pcrProfile); err != nil {
		return fmt.Errorf("cannot update PCR profile: %w", err)
	}

	w := secboot_tpm2.NewFileSealedKeyObjectWriter(filepath.Join(esp, keyFilePath))
	if err := sbtpmSealedKeyObjectWriteAtomic(k, w); err != nil {
		return fmt.Errorf("cannot write updated sealed key object: %w", err)
	}

	return nil
}
