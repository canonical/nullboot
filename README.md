# A boot manager that isn't

nullboot is a boot manager for environments that do not need a boot manager.
Instead of running a boot manager at boot, it directly manages the UEFI boot
entries for you.

## Use cases

Usage:
```
1. nullbootctl install      [OPTIONS]
2. nullbootctl recovery-key [OPTIONS]
3. nullbootctl encrypt      [OPTIONS]
4. nullbootctl deploy       [OPTIONS]
```

### nullbootctl install

Install shim and UKI in EFI System Partition.

More specifically:
- Install shim and UKI (Unified Kernel Image) to the EFI System Partition (ESP,
  ie: /boot/efi)
- Update the TPM sealing policy of the LUKS passphrase
- Set EFI variable BootNext to point to the newly installed shim (and kernel?) (TODO)

It is intended to be called upon shim and UKI update.

See `nullbootctl install --help` for options:
```
Usage:
  nullbootctl install [OPTIONS]

Help Options:
  -h, --help                      Show this help message

Options:
          --no-tpm                Do not do any resealing with the TPM
          --no-efivars            Do not use or update the EFI variables.
                                  Disables kernel fallback mechanism
          --output-json=<file>    JSON file to write. Disables writing real EFI
                                  variables and enablement of the kernel
                                  fallback mechanism
          --no-boot-next          Disables use of BootNext. This flag must be
                                  disabled in order to upgrade to a new kernel
                                  version.
```


### nullbootctl recovery-key

Manage recovery keys (LUKS passphrases) of a LUKS container.

This shall setup, list, delete and verify recovery keys for a LUKS container.

See `nullbootctl recovery-key --help` for options:
```
Usage:
  nullbootctl recovery-key [OPTIONS]

Create, list, delete or verify a LUKS2 disk encryption recovery key.

Help Options:
  -h, --help                   Show this help message

Options:
          --create             Create and set a recovery key
          --list               List recovery keys
          --delete             Delete a recovery key
          --verify             Verify a recovery key
          --device=<device>    Device of the encrypted volume (default:
                               /dev/disk/by-label/cloudimg-rootfs-enc)
          --name=<name>        Name of the recovery key (default: recovery-0001)
```

Example:
```
# nullbootctl recovery-key --create --name R1
Creating recovery key 'recovery-0001' in '/dev/disk/by-label/cloudimg-rootfs-enc'
38785-16377-60025-09490-45202-37955-36799-29994

# nullbootctl recovery-key --create --name R2
Creating recovery key 'R2' in '/dev/disk/by-label/cloudimg-rootfs-enc'
43392-00597-12755-38614-06919-19535-19552-05646

# nullbootctl recovery-key --list
Listing recovery keys in '/dev/disk/by-label/cloudimg-rootfs-enc'
R1
R2

# echo 38785-16377-60025-09490-45202-37955-36799-29994 | nullbootctl recovery-key --verify
success
```

### nullbootctl encrypt

Perform the pre-encryption of a Ubuntu cloud image without protecting the key
to a specific guest instance. The key is stored in cleartext inside the LUKS
metadata, and can be used to create a pool of pre-encrypted images.

See `nullbootctl encrypt --help` for options:
```
Usage:
  nullbootctl encrypt [OPTIONS] Source image path (file or block device)

Help Options:
  -h, --help                            Show this help message

Options:
      -v, --verbose                     Enable verbose debug output
      -o, --output=                     Output image path
          --kernel-efi=                 Path to kernel.efi for booting
          --override-datasources=       Override the cloud-init datasources with the
                                        supplied comma-delimited list of sources
          --grow-root                   Grow the root partition to fill the available
                                        space, disabling cloud-init's cc_growpart
          --root-partition-uuid=        Explicitly provide the root partition UUID
                                        instead of relying on partition type
          --efi-partition-uuid=         Explicitly provide the ESP partition UUID instead
                                        of relying on partition type
```

### nullbootctl deploy

Take a pre-encrypted image and protect the key to a virtual TPM associated with
a specific guest instance, removing the cleartext key in the process. Once this
step has been performed, the image is only bootable on that specific guest
instance. The deploy step does not need access to the actual TPM, but does need
access to the public area of the storage primary key.

See `nullbootctl deploy --help` for options:
```
Usage:
  nullbootctl deploy [OPTIONS] Source image path (file or block device)

Help Options:
  -h, --help                                Show this help message

Options:
      -v, --verbose                         Enable verbose debug output
          --recovery-key-file=              Add a recovery key from the supplied file. The key
                                            must be 16-bytes long
          --add-efi-boot-manager-profile    Protect the disk unlock key with the EFI boot
                                            manager code and boot attempts profile (PCR4)
          --add-efi-secure-boot-profile     Protect the disk unlock key with the EFI secure
                                            boot policy profile (PCR7)
          --add-ubuntu-kernel-profile       Protect the disk unlock key with properties
                                            measured by the Ubuntu kernel (PCR12). Also
                                            prevents access outside of early boot
          --root-partition-uuid=            Explicitly provide the root partition UUID instead
                                            of relying on partition type
          --efi-partition-uuid=             Explicitly provide the ESP partition UUID instead
                                            of relying on partition type
          --az-disk-profile=
          --uefi-config=                    JSON file describring the platform firmware
                                            configuration
          --srk-template-unique-data=       Path to the TPMU_PUBLIC_ID structure used to
                                            create the SRK
          --srk-pub=                        Path to SRK public area
          --standard-srk-template           Indicate that the supplied SRK was created with
                                            the TCG TPM v2.0 Provisioning Guidance spec
```



## Build
```
$ go build -o . ./...
```

## Testing

### Unit tests

```
go test -v -coverprofile=profile.cov ./...
```

### Test encrypt & deploy

See [README-test-encrypt-deploy.md](README-test-encrypt-deploy.md).


## Licensing
This program is free software: you can redistribute it and/or modify it under
the terms of the GNU General Public License version 3, as published by the Free
Software Foundation.

This program is distributed in the hope that it will be useful, but WITHOUT ANY
WARRANTY; without even the implied warranties of MERCHANTABILITY, SATISFACTORY
QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public
License for more details.

You should have received a copy of the GNU General Public License along with
this program.  If not, see <http://www.gnu.org/licenses/>.
