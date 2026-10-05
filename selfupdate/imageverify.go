package selfupdate

import (
	"context"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"fmt"
	"io"
	"runtime"
)

type imageFormat uint8

const (
	formatELF imageFormat = iota + 1
	formatMachO
	formatPE
)

type imageVerifier struct {
	platform  Platform
	format    imageFormat
	elfClass  elf.Class
	elfMach   elf.Machine
	machoCPU  macho.Cpu
	peMachine uint16
}

// NewImageVerifier returns a Verifier that requires the staged binary to be
// an executable image for p: ELF for linux, freebsd, netbsd, openbsd and
// dragonfly, Mach-O (thin, or fat with a matching arch) for darwin, and PE
// for windows; for amd64, arm64, 386 or arm. A zero p means the runtime
// platform. ELF OSABI is not checked: Go's linker writes ELFOSABI_NONE for
// linux, so it cannot tell operating systems apart (0004-MADR G9).
func NewImageVerifier(p Platform) (Verifier, error) {
	v, err := imageCheckFor(p)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// CheckImage is NewImageVerifier's check on r directly: it requires r to
// hold an executable image for p, and fails wrapping ErrIntegrity when it
// does not. A zero p means the runtime platform, and a platform the check
// does not cover fails with ErrUnsupportedPlatform. An Unpacker uses it on
// the program it extracted (0012-MADR §2, §4).
func CheckImage(r io.ReaderAt, p Platform) error {
	v, err := imageCheckFor(p)
	if err != nil {
		return err
	}
	if !v.matches(r) {
		return fmt.Errorf("selfupdate: the program is not a %s/%s executable: %w",
			sanitizeText(v.platform.OS), sanitizeText(v.platform.Arch), ErrIntegrity)
	}
	return nil
}

func imageCheckFor(p Platform) (imageVerifier, error) {
	if p == (Platform{}) {
		p = Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	}
	v := imageVerifier{platform: p}
	switch p.OS {
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly":
		v.format = formatELF
	case "darwin":
		v.format = formatMachO
	case goosWindows:
		v.format = formatPE
	}
	switch p.Arch {
	case "amd64":
		v.elfClass, v.elfMach, v.machoCPU, v.peMachine = elf.ELFCLASS64, elf.EM_X86_64, macho.CpuAmd64, pe.IMAGE_FILE_MACHINE_AMD64
	case "arm64":
		v.elfClass, v.elfMach, v.machoCPU, v.peMachine = elf.ELFCLASS64, elf.EM_AARCH64, macho.CpuArm64, pe.IMAGE_FILE_MACHINE_ARM64
	case "386":
		v.elfClass, v.elfMach, v.machoCPU, v.peMachine = elf.ELFCLASS32, elf.EM_386, macho.Cpu386, pe.IMAGE_FILE_MACHINE_I386
	case "arm":
		v.elfClass, v.elfMach, v.machoCPU, v.peMachine = elf.ELFCLASS32, elf.EM_ARM, macho.CpuArm, pe.IMAGE_FILE_MACHINE_ARMNT
	default:
		v.format = 0
	}
	if v.format == 0 {
		return imageVerifier{}, fmt.Errorf("selfupdate: no executable image check for %s/%s: %w",
			sanitizeText(p.OS), sanitizeText(p.Arch), ErrUnsupportedPlatform)
	}
	return v, nil
}

func (v imageVerifier) Verify(_ context.Context, ver Verification) (err error) {
	if ver.Open == nil {
		return fmt.Errorf("selfupdate: image verifier needs the staged file")
	}
	rc, err := ver.Open()
	if err != nil {
		return err
	}
	defer func() {
		err = joinClose(err, rc)
	}()
	ra, ok := rc.(io.ReaderAt)
	if !ok {
		return fmt.Errorf("selfupdate: image verifier needs a seekable staged file")
	}
	if !v.matches(ra) {
		return fmt.Errorf("selfupdate: staged binary is not a %s/%s executable: %w",
			sanitizeText(v.platform.OS), sanitizeText(v.platform.Arch), ErrIntegrity)
	}
	return nil
}

func (v imageVerifier) matches(ra io.ReaderAt) bool {
	switch v.format {
	case formatELF:
		f, err := elf.NewFile(ra)
		// A Go executable is ET_EXEC, or ET_DYN when built as a PIE.
		return err == nil && f.Class == v.elfClass && f.Machine == v.elfMach &&
			(f.Type == elf.ET_EXEC || f.Type == elf.ET_DYN)
	case formatMachO:
		ff, err := macho.NewFatFile(ra)
		if err == nil {
			for _, a := range ff.Arches {
				if a.Cpu == v.machoCPU && a.Type == macho.TypeExec {
					return true
				}
			}
			return false
		}
		if !errors.Is(err, macho.ErrNotFat) {
			return false
		}
		f, err := macho.NewFile(ra)
		return err == nil && f.Cpu == v.machoCPU && f.Type == macho.TypeExec
	case formatPE:
		f, err := pe.NewFile(ra)
		return err == nil && f.Machine == v.peMachine && f.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE != 0
	}
	return false
}
