//go:build windows

package scm

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// sysManager is the real SCM. Each open connects with SC_MANAGER_CONNECT
// alone and opens the service with the caller's rights, wrapping both
// handles in mgr.Mgr and mgr.Service: mgr.Connect would ask for
// SC_MANAGER_ALL_ACCESS (0011-MADR §7).
type sysManager struct{}

func systemManager() manager { return sysManager{} }

func (sysManager) open(name string, access uint32) (handle, error) {
	mh, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, err
	}
	m := &mgr.Mgr{Handle: mh}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, errors.Join(err, m.Disconnect())
	}
	sh, err := windows.OpenService(m.Handle, p, access)
	if err != nil {
		return nil, errors.Join(err, m.Disconnect())
	}
	return &sysHandle{m: m, s: &mgr.Service{Name: name, Handle: sh}}, nil
}

func (sysManager) processes() (procs []process, err error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(snap)) }()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e)) //nolint:gosec // the size of a fixed struct
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		procs = append(procs, process{PID: e.ProcessID, ParentPID: e.ParentProcessID})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return procs, nil
}

func (sysManager) created(pid uint32) uint64 {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0
	}
	var created, exited, kernel, user windows.Filetime
	err = windows.GetProcessTimes(h, &created, &exited, &kernel, &user)
	if err = errors.Join(err, windows.CloseHandle(h)); err != nil {
		return 0
	}
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
}

func (sysManager) decompose(cmdline string) ([]string, error) {
	return windows.DecomposeCommandLine(cmdline)
}

func (sysManager) compose(args []string) string { return windows.ComposeCommandLine(args) }

type sysHandle struct {
	m *mgr.Mgr
	s *mgr.Service
}

// status reads SERVICE_STATUS_PROCESS directly: mgr.Service.Query leaves
// out the checkpoint and wait hint the polling loop needs.
func (h *sysHandle) status() (status, error) {
	var t windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if err := windows.QueryServiceStatusEx(h.s.Handle, windows.SC_STATUS_PROCESS_INFO,
		(*byte)(unsafe.Pointer(&t)), uint32(unsafe.Sizeof(t)), &needed); err != nil { //nolint:gosec // SERVICE_STATUS_PROCESS for QueryServiceStatusEx
		return status{}, err
	}
	return status{
		Type: t.ServiceType, State: t.CurrentState, CheckPoint: t.CheckPoint, WaitHint: t.WaitHint,
		ProcessID: t.ProcessId, Win32ExitCode: t.Win32ExitCode, SpecificExit: t.ServiceSpecificExitCode,
	}, nil
}

func (h *sysHandle) control(code uint32) (status, error) {
	st, err := h.s.Control(svc.Cmd(code))
	return status{State: uint32(st.State)}, err
}

func (h *sysHandle) start() error { return h.s.Start() }

func (h *sysHandle) config() (config, error) {
	c, err := h.s.Config()
	if err != nil {
		return config{}, err
	}
	triggers, err := h.triggers()
	if err != nil {
		return config{}, err
	}
	return config{
		Type: c.ServiceType, StartType: c.StartType, DelayedAutoStart: c.DelayedAutoStart,
		BinaryPathName: c.BinaryPathName, Triggers: triggers,
	}, nil
}

// triggers reads SERVICE_TRIGGER_INFO's count, its first DWORD.
func (h *sysHandle) triggers() (uint32, error) {
	n := uint32(1024)
	for {
		b := make([]byte, n)
		err := windows.QueryServiceConfig2(h.s.Handle, windows.SERVICE_CONFIG_TRIGGER_INFO, &b[0], n, &n)
		if err == nil {
			return *(*uint32)(unsafe.Pointer(&b[0])), nil //nolint:gosec // SERVICE_TRIGGER_INFO's first DWORD
		}
		if !errors.Is(err, syscall.ERROR_INSUFFICIENT_BUFFER) || int(n) <= len(b) {
			return 0, err
		}
	}
}

func (h *sysHandle) setBinaryPathName(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.ChangeServiceConfig(h.s.Handle, windows.SERVICE_NO_CHANGE, windows.SERVICE_NO_CHANGE,
		windows.SERVICE_NO_CHANGE, p, nil, nil, nil, nil, nil, nil)
}

func (h *sysHandle) dependents() ([]string, error) {
	return h.s.ListDependentServices(svc.Active)
}

func (h *sysHandle) close() error {
	return errors.Join(h.s.Close(), h.m.Disconnect())
}
