A boot manager that isn't
=========================

nullboot is a boot manager for environments that do not need a boot manager.
Instead of running a boot manager at boot, it directly manages the UEFI boot
entries for you.

Use cases
---------

Usage:
1. nullbootctl
2. nullbootctl -no-tpm -output-json FILE
3. nullbootctl -no-boot-next
4. nullbootctl recovery-passphrase [OPTIONS]

1. nullbootctl

Finalize shim and UKI installation.

Without any argument, nullbootctl shall:
- install shim and UKI (Unified Kernel Image) to the EFI System Partition (ESP,
  ie: /boot/efi)
- update the sealing policy of the LUKS passphrase in the TPM
- set EFI variable BootNext to point to the newly installed shim and kernel

It is intended to be called upon shim and UKI update.


2. nullbootctl -no-tpm -output-json FILE

Export EFI variables Boot#### and BootOrder.

Install shim & UKI ? => check.

This shall create a JSON file with the EFI variables Boot#### and BootOrder, as
expected on the first boot.


3. nullbootctl -no-boot-next

Commit EFI variable BootOrder after shim and UKI update.

This shall:
- install shim and UKI, but this is generally a no-operation
- update the sealing policy of the LUKS passphrase in the TPM (generally a
  no-operation)
- commit shim and kernel by updating EFI variable BootOrder (when the system
  boots on EFI variable BootNext) - remove old kernel?


4. nullbootctl recovery-passphrase [OPTIONS]

Manage recovery passphrase of a LUKS container.

This shall setup, list or delete a recovery passphrase for the LUKS container.

OPTIONS
  --delete NAME  delete a recovery passphrase
  --list         list passphrases
  --setup        create a random recovery passphrase and add it to the LUKS
                 container
  --device       default to  ...


Build
-----
```
$ go build -o . ./...
```

Execute
-------
```
$ ./nullbootctl -h
Usage of ./build/nullbootctl:
  -no-boot-next
    	Disables use of BootNext. This flag must be disabled in order to upgrade to a new kernel version.
  -no-efivars
    	Do not use or update the EFI variables. Disables kernel fallback mechanism
  -no-tpm
    	Do not do any resealing with the TPM
  -output-json string
    	JSON file to write. Disables writing real EFI variables and enablement of the kernel fallback mechanism
```

Unit test
---------
```
$ go test -v -coverprofile=profile.cov ./... 
$ echo $?
0
```

Licensing
---------
This program is free software: you can redistribute it and/or modify it under
the terms of the GNU General Public License version 3, as published by the Free
Software Foundation.

This program is distributed in the hope that it will be useful, but WITHOUT ANY
WARRANTY; without even the implied warranties of MERCHANTABILITY, SATISFACTORY
QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public
License for more details.

You should have received a copy of the GNU General Public License along with
this program.  If not, see <http://www.gnu.org/licenses/>.
