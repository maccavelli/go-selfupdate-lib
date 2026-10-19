module github.com/maccavelli/go-selfupdate-lib

go 1.26.9

toolchain go1.27.2

retract [v1.0.0, v1.4.1] // declared github.com/maccavelli/go-core-lib; use v1.5.0 or later

require (
	golang.org/x/mod v0.40.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.43.0
)
