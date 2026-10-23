//go:build windows

package taskscheduler

import (
	"errors"
	"math"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CLSID and IIDs from taskschd.idl, each the uuid on that interface.
var (
	clsidTaskScheduler = windows.GUID{Data1: 0x0f87369f, Data2: 0xa4e5, Data3: 0x4cfc, Data4: [8]byte{0xbd, 0x3e, 0x73, 0xe6, 0x15, 0x45, 0x72, 0xdd}}
	iidITaskService    = windows.GUID{Data1: 0x2faba4c7, Data2: 0x4da9, Data3: 0x4013, Data4: [8]byte{0x96, 0x97, 0x20, 0xcc, 0x3f, 0xd4, 0x0f, 0x85}}
	iidIExecAction     = windows.GUID{Data1: 0x4c3d624d, Data2: 0xfd6b, Data3: 0x49a3, Data4: [8]byte{0xb9, 0xb7, 0x09, 0xcb, 0x3c, 0xd3, 0xf0, 0x47}}
)

const (
	hrSFalse         = hresult(1)
	hrRPCChangedMode = hresult(0x80010106) // RPC_E_CHANGED_MODE
	vtI4             = 3
	variantTrue      = uint16(0xFFFF)
	taskUpdate       = 0x4  // TASK_UPDATE
	taskIgnoreReg    = 0x20 // TASK_IGNORE_REGISTRATION_TRIGGERS
	taskUpdateFlags  = taskUpdate | taskIgnoreReg
)

func (h hresult) failed() bool { return uint32(h)&0x80000000 != 0 }

func ptr[T any](v *T) uintptr {
	return uintptr(unsafe.Pointer(v)) //nolint:gosec // COM argument
}

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	oleaut32             = windows.NewLazySystemDLL("oleaut32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procSysAllocString   = oleaut32.NewProc("SysAllocString")
	procSysFreeString    = oleaut32.NewProc("SysFreeString")
)

// lastCoInit is the HRESULT of the most recent CoInitializeEx, for
// TestCOMLayout. It is atomic because every COM call writes it, from any
// goroutine (0025-MADR-align-with-go-sdk-design-practices.md D4).
var lastCoInit atomic.Uint32

// IUnknown + IDispatch occupy slots 0-6. Interface methods start at 7
// (0021-MADR Amendment A3). Values are absolute vtable indexes.
const (
	slotRelease = 2

	// ITaskService, after IDispatch.
	svcGetFolder = 7  // GetFolder
	svcConnect   = 10 // Connect

	// ITaskFolder, after IDispatch. CreateFolder and GetTasks are taken
	// from MinGW-w64's taskschd.h and pinned by TestCOMRegistration
	// (0032-MADR F16).
	folderCreateFolder           = 11 // CreateFolder
	folderGetTask                = 13 // GetTask
	folderGetTasks               = 14 // GetTasks
	folderDeleteTask             = 15 // DeleteTask
	folderRegisterTask           = 16 // RegisterTask
	folderRegisterTaskDefinition = 17 // RegisterTaskDefinition
	taskEnumHidden               = 1  // TASK_ENUM_HIDDEN

	// IRegisteredTask, after IDispatch.
	taskGetName        = 7  // get_Name
	taskGetState       = 9  // get_State
	taskGetEnabled     = 10 // get_Enabled
	taskPutEnabled     = 11 // put_Enabled
	taskRun            = 12 // Run
	taskGetInstances   = 14 // GetInstances
	taskGetLastRunTime = 15 // get_LastRunTime
	taskGetLastResult  = 16 // get_LastTaskResult
	taskGetDefinition  = 19 // get_Definition
	taskGetXML         = 20 // get_Xml
	taskStop           = 23 // Stop

	// IRunningTaskCollection / IActionCollection / ITriggerCollection.
	colGetCount = 7 // get_Count
	colGetItem  = 8 // get_Item

	// IRunningTask, after IDispatch.
	runningGetEnginePID = 14 // get_EnginePID

	// ITaskDefinition, after IDispatch.
	defGetTriggers  = 9  // get_Triggers
	defGetPrincipal = 15 // get_Principal
	defGetActions   = 17 // get_Actions

	// IAction, after IDispatch.
	actionGetType = 9 // get_Type

	// IExecAction, after IAction.
	execGetPath      = 10 // get_Path
	execPutPath      = 11 // put_Path
	execGetArguments = 12 // get_Arguments

	// ITrigger, after IDispatch.
	trigGetType       = 7  // get_Type
	trigGetRepetition = 10 // get_Repetition
	trigGetEnabled    = 18 // get_Enabled

	// IRepetitionPattern, after IDispatch.
	repGetInterval = 7 // get_Interval

	// IPrincipal, after IDispatch. get_UserId returns the account's name
	// without its domain (0032-MADR Amendment A1, P2-6).
	prinGetUserID    = 11 // get_UserId
	prinGetLogonType = 13 // get_LogonType
	prinGetRunLevel  = 17 // get_RunLevel
)

// variant is VARIANT on amd64/arm64 (24 bytes). TestVARIANTSize pins it.
type variant struct {
	vt   uint16
	_    [3]uint16
	data [16]byte
}

type comObj struct {
	vtbl *[32]uintptr
}

func (o *comObj) call(slot int, args ...uintptr) hresult {
	all := make([]uintptr, 1+len(args))
	all[0] = ptr(o)
	copy(all[1:], args)
	r, _, _ := syscall.SyscallN(o.vtbl[slot], all...) //nolint:errcheck // HRESULT is r, not last-error
	runtime.KeepAlive(o)
	return hresult(uint32(r)) //nolint:gosec // HRESULT is the low 32 bits
}

func (o *comObj) release() {
	if o == nil {
		return
	}
	_, _, _ = syscall.SyscallN(o.vtbl[slotRelease], ptr(o)) //nolint:errcheck // Release's ULONG is ignored
}

func (o *comObj) query(iid *windows.GUID) (*comObj, error) {
	var out *comObj
	hr := o.call(0, ptr(iid), ptr(&out))
	if hr.failed() {
		return nil, hr
	}
	return out, nil
}

func withCOM(fn func() error) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r, _, _ := procCoInitializeEx.Call(0, uintptr(windows.COINIT_MULTITHREADED)) //nolint:errcheck // HRESULT is r
	hr := hresult(uint32(r))                                                     //nolint:gosec // HRESULT is the low 32 bits
	lastCoInit.Store(uint32(hr))
	switch {
	case !hr.failed():
		defer windows.CoUninitialize()
	case hr == hrRPCChangedMode:
		// Thread already has COM in the other apartment; do not uninit.
	default:
		return hr
	}
	return fn()
}

func withService(fn func(svc *comObj) error) error {
	return withCOM(func() error {
		var svc *comObj
		r, _, _ := procCoCreateInstance.Call( //nolint:errcheck // HRESULT is r
			ptr(&clsidTaskScheduler),
			0,
			uintptr(windows.CLSCTX_INPROC_SERVER),
			ptr(&iidITaskService),
			ptr(&svc),
		)
		if hr := hresult(uint32(r)); hr.failed() { //nolint:gosec // HRESULT is the low 32 bits
			return hr
		}
		if svc == nil {
			return errors.New("selfupdate: taskscheduler: CoCreateInstance returned nil")
		}
		defer svc.release()
		var empty [4]variant
		if hr := svc.call(svcConnect,
			ptr(&empty[0]),
			ptr(&empty[1]),
			ptr(&empty[2]),
			ptr(&empty[3]),
		); hr.failed() {
			return hr
		}
		return fn(svc)
	})
}

func withFolder(folder string, fn func(f *comObj) error) error {
	return withService(func(svc *comObj) error {
		f, err := getFolder(svc, folder)
		if err != nil {
			return err
		}
		defer f.release()
		return fn(f)
	})
}

func withTask(folder, name string, fn func(task, folderObj *comObj) error) error {
	return withFolder(folder, func(f *comObj) error {
		t, err := getTask(f, name)
		if err != nil {
			return err
		}
		defer t.release()
		return fn(t, f)
	})
}

func getFolder(svc *comObj, path string) (*comObj, error) {
	b, err := sysString(path)
	if err != nil {
		return nil, err
	}
	defer freeBSTR(b)
	var f *comObj
	if hr := svc.call(svcGetFolder, b, ptr(&f)); hr.failed() {
		return nil, hr
	}
	return f, nil
}

func getTask(folder *comObj, name string) (*comObj, error) {
	b, err := sysString(name)
	if err != nil {
		return nil, err
	}
	defer freeBSTR(b)
	var t *comObj
	if hr := folder.call(folderGetTask, b, ptr(&t)); hr.failed() {
		return nil, hr
	}
	return t, nil
}

func sysString(s string) (uintptr, error) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	r, _, _ := procSysAllocString.Call(ptr(&u[0])) //nolint:errcheck // NULL return is checked below
	runtime.KeepAlive(u)
	if r == 0 && s != "" {
		return 0, errors.New("selfupdate: taskscheduler: SysAllocString failed")
	}
	return r, nil
}

func freeBSTR(b uintptr) {
	_, _, _ = procSysFreeString.Call(b) //nolint:errcheck // SysFreeString returns void
}

func getBSTR(o *comObj, slot int) (string, error) {
	var b *uint16
	if hr := o.call(slot, ptr(&b)); hr.failed() {
		return "", hr
	}
	if b == nil {
		return "", nil
	}
	s := windows.UTF16PtrToString(b)
	freeBSTR(uintptr(unsafe.Pointer(b))) //nolint:gosec // BSTR returned by COM
	return s, nil
}

func getLong(o *comObj, slot int) (int32, error) {
	var n int32
	if hr := o.call(slot, ptr(&n)); hr.failed() {
		return 0, hr
	}
	return n, nil
}

func oleDate(d float64) time.Time {
	days, frac := math.Modf(d)
	sec := frac * 86400
	return time.Date(1899, 12, 30, 0, 0, 0, 0, time.Local).
		AddDate(0, 0, int(days)).
		Add(time.Duration(sec * float64(time.Second)))
}

func variantI4(n int32) variant {
	v := variant{vt: vtI4}
	*(*int32)(unsafe.Pointer(&v.data[0])) = n //nolint:gosec // VARIANT union, VT_I4 at offset 0 of data
	return v
}

func boolArg(on bool) uintptr {
	if on {
		return uintptr(variantTrue)
	}
	return 0
}

func checkHR(hr hresult) error {
	if hr.failed() {
		return hr
	}
	return nil
}
