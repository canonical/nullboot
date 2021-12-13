module github.com/canonical/nullboot

go 1.16

require (
	github.com/canonical/go-efilib v0.2.0
	github.com/canonical/go-sp800.90a-drbg v0.0.0-20210314144037-6eeb1040d6c3 // indirect
	github.com/canonical/go-tpm2 v0.1.0
	github.com/canonical/tcglog-parser v0.0.0-20210824131805-69fa1e9f0ad2
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/knqyf263/go-deb-version v0.0.0-20190517075300-09fca494f03d
	github.com/snapcore/secboot v0.0.0-20211029143450-8cdfc8e774d0
	github.com/snapcore/snapd v0.0.0-20210902070205-9fe87efa1b36 // indirect
	github.com/spf13/afero v1.6.0
	golang.org/x/sys v0.0.0-20211031064116-611d5d643895
	golang.org/x/text v0.3.7
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/retry.v1 v1.0.3 // indirect
	maze.io/x/crypto v0.0.0-20190131090603-9b94c9afe066 // indirect
)

replace github.com/snapcore/secboot => github.com/chrisccoulson/secboot v0.0.0-20211101133820-41f32b803753
