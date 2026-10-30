module github.com/maccavelli/go-selfupdate-lib

go 1.26.10

toolchain go1.27.3

retract [v1.0.0, v1.4.1] // declared github.com/maccavelli/go-core-lib; use v1.5.0 or later

require (
	golang.org/x/mod v0.42.0
	golang.org/x/sys v0.49.0
	golang.org/x/term v0.47.0
)
