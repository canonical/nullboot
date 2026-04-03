A boot manager that isn't
=========================

nullboot is a boot manager for environments that do not need a boot manager.
Instead of running a boot manager at boot, it directly manages the UEFI boot
entries for you.

Build
-----
```
$ mkdir build
$ go build -o build ./...
```

Execute
-------
```
$ build/nullbootctl -h
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
