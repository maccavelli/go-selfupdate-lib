//go:build windows

package taskscheduler

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

type sysSched struct{}

func systemScheduler() scheduler { return sysSched{} }

func (sysSched) open(folder, name string) (registered, error) {
	err := withTask(folder, name, func(*comObj, *comObj) error { return nil })
	if err != nil {
		return nil, err
	}
	return &sysReg{folder: folder, name: name}, nil
}

func (s sysSched) processes() (procs []winproc.Process, err error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(snap)) }()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e)) //nolint:gosec // the size of a fixed struct
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		procs = append(procs, winproc.Process{PID: e.ProcessID, ParentPID: e.ParentProcessID})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return procs, nil
}

func (sysSched) created(pid uint32) uint64 {
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

func (sysSched) register(folder, name, xml string, flags, logon int32) error {
	return withFolder(folder, func(f *comObj) error {
		nb, err := sysString(name)
		if err != nil {
			return err
		}
		defer freeBSTR(nb)
		xb, err := sysString(xml)
		if err != nil {
			return err
		}
		defer freeBSTR(xb)
		var empty [3]variant
		var out *comObj
		hr := f.call(folderRegisterTask,
			nb,
			xb,
			uintptr(uint32(flags)), //nolint:gosec // TASK_CREATION is a small flag set
			ptr(&empty[0]),
			ptr(&empty[1]),
			uintptr(uint32(logon)), //nolint:gosec // TASK_LOGON_TYPE is a small enum
			ptr(&empty[2]),
			ptr(&out),
		)
		out.release()
		return checkHR(hr)
	})
}

func (sysSched) deleteTask(folder, name string) error {
	return withFolder(folder, func(f *comObj) error {
		nb, err := sysString(name)
		if err != nil {
			return err
		}
		defer freeBSTR(nb)
		return checkHR(f.call(folderDeleteTask, nb, 0))
	})
}

func (sysSched) createFolder(path string) error {
	return withFolder(`\`, func(root *comObj) error {
		b, err := sysString(strings.TrimPrefix(path, `\`))
		if err != nil {
			return err
		}
		defer freeBSTR(b)
		var sddl variant
		var out *comObj
		hr := root.call(folderCreateFolder, b, ptr(&sddl), ptr(&out))
		out.release()
		return checkHR(hr)
	})
}

func (sysSched) list(folder string) ([]listed, error) {
	var out []listed
	err := withFolder(folder, func(f *comObj) error {
		var col *comObj
		if hr := f.call(folderGetTasks, taskEnumHidden, ptr(&col)); hr.failed() {
			return hr
		}
		if col == nil {
			return nil
		}
		defer col.release()
		n, err := getLong(col, colGetCount)
		if err != nil {
			return err
		}
		for i := int32(1); i <= n; i++ {
			idx := variantI4(i)
			var t *comObj
			if hr := col.call(colGetItem, ptr(&idx), ptr(&t)); hr.failed() {
				return hr
			}
			if t == nil {
				continue
			}
			l, err := listEntry(t)
			t.release()
			if err != nil {
				return err
			}
			out = append(out, l)
		}
		return nil
	})
	return out, err
}

// listEntry reads one registered task of a listing.
func listEntry(t *comObj) (listed, error) {
	var l listed
	var err error
	if l.Name, err = getBSTR(t, taskGetName); err != nil {
		return l, err
	}
	if l.State, err = getLong(t, taskGetState); err != nil {
		return l, err
	}
	if l.LastResult, err = getLong(t, taskGetLastResult); err != nil {
		return l, err
	}
	var d float64
	if hr := t.call(taskGetLastRunTime, ptr(&d)); hr.failed() {
		return l, hr
	}
	l.LastRun = oleDate(d)
	return l, nil
}

// running is winproc.Alive: a process that has exited reads as gone even
// while another process holds a handle to it, which a creation-time check
// alone missed (0022-MADR Amendment A2).
func (sysSched) running(pid uint32, born uint64) bool {
	return winproc.Alive(pid, born)
}

type sysReg struct {
	folder, name string
}

func (r *sysReg) close() error { return nil }

func (r *sysReg) state() (int32, error) {
	var st int32
	err := withTask(r.folder, r.name, func(t, _ *comObj) error {
		n, err := getLong(t, taskGetState)
		st = n
		return err
	})
	return st, err
}

func (r *sysReg) enabled() (bool, error) {
	var on bool
	err := withTask(r.folder, r.name, func(t, _ *comObj) error {
		var b int16
		if hr := t.call(taskGetEnabled, ptr(&b)); hr.failed() {
			return hr
		}
		on = b != 0
		return nil
	})
	return on, err
}

func (r *sysReg) setEnabled(on bool) error {
	return withTask(r.folder, r.name, func(t, _ *comObj) error {
		if hr := t.call(taskPutEnabled, boolArg(on)); hr.failed() {
			return hr
		}
		return nil
	})
}

func (r *sysReg) run() error {
	return withTask(r.folder, r.name, func(t, _ *comObj) error {
		var empty variant
		var running *comObj
		if hr := t.call(taskRun, ptr(&empty), ptr(&running)); hr.failed() {
			return hr
		}
		running.release()
		return nil
	})
}

func (r *sysReg) stop() error {
	return withTask(r.folder, r.name, func(t, _ *comObj) error {
		hr := t.call(taskStop, 0)
		if hr.failed() {
			return hr
		}
		if hr != hrSFalse {
			return nil
		}
		// S_FALSE: the caller cannot stop running instances, or none remain
		// (0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md T6:
		// RequestStop already ended the process).
		var col *comObj
		if ihr := t.call(taskGetInstances, 0, ptr(&col)); ihr.failed() {
			if uint32(ihr) == uint32(hrTaskNotRunning) {
				return nil
			}
			return fmt.Errorf("selfupdate: taskscheduler: cannot stop instances of \\%s: %w", r.name, hr)
		}
		if col == nil {
			return nil
		}
		defer col.release()
		n, err := getLong(col, colGetCount)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		return fmt.Errorf("selfupdate: taskscheduler: cannot stop instances of \\%s", r.name)
	})
}

func (r *sysReg) instances() ([]uint32, error) {
	var pids []uint32
	err := withTask(r.folder, r.name, func(t, _ *comObj) error {
		var col *comObj
		if hr := t.call(taskGetInstances, 0, ptr(&col)); hr.failed() {
			if uint32(hr) == uint32(hrTaskNotRunning) {
				return nil
			}
			return hr
		}
		if col == nil {
			return nil
		}
		defer col.release()
		n, err := getLong(col, colGetCount)
		if err != nil {
			return err
		}
		for i := int32(1); i <= n; i++ {
			idx := variantI4(i)
			var inst *comObj
			if hr := col.call(colGetItem, ptr(&idx), ptr(&inst)); hr.failed() {
				return hr
			}
			if inst == nil {
				continue
			}
			var pid uint32
			hr := inst.call(runningGetEnginePID, ptr(&pid))
			inst.release()
			if hr.failed() {
				return hr
			}
			if pid != 0 {
				pids = append(pids, pid)
			}
		}
		return nil
	})
	return pids, err
}

func (r *sysReg) xml() (string, error) {
	var s string
	err := withTask(r.folder, r.name, func(t, _ *comObj) error {
		v, err := getBSTR(t, taskGetXML)
		s = v
		return err
	})
	return s, err
}

func (r *sysReg) lastRun() (lastRun, error) {
	var out lastRun
	err := withTask(r.folder, r.name, func(t, _ *comObj) error {
		var d float64
		if hr := t.call(taskGetLastRunTime, ptr(&d)); hr.failed() {
			return hr
		}
		n, err := getLong(t, taskGetLastResult)
		if err != nil {
			return err
		}
		out.Time, out.Result = oleDate(d), n
		return nil
	})
	return out, err
}

func (r *sysReg) logonType() (int32, error) {
	var n int32
	err := withTask(r.folder, r.name, func(t, _ *comObj) error {
		def, err := definition(t)
		if err != nil {
			return err
		}
		defer def.release()
		prin, err := getObj(def, defGetPrincipal)
		if err != nil {
			return err
		}
		defer prin.release()
		n, err = getLong(prin, prinGetLogonType)
		return err
	})
	return n, err
}

func (r *sysReg) principal() (principal, error) {
	var p principal
	err := withTask(r.folder, r.name, func(t, _ *comObj) error {
		def, err := definition(t)
		if err != nil {
			return err
		}
		defer def.release()
		prin, err := getObj(def, defGetPrincipal)
		if err != nil {
			return err
		}
		defer prin.release()
		if p.UserID, err = getBSTR(prin, prinGetUserID); err != nil {
			return err
		}
		if p.LogonType, err = getLong(prin, prinGetLogonType); err != nil {
			return err
		}
		p.RunLevel, err = getLong(prin, prinGetRunLevel)
		return err
	})
	return p, err
}

func (r *sysReg) actions() ([]action, error) {
	var out []action
	err := withTask(r.folder, r.name, func(t, _ *comObj) error {
		def, err := definition(t)
		if err != nil {
			return err
		}
		defer def.release()
		col, err := getObj(def, defGetActions)
		if err != nil {
			return err
		}
		defer col.release()
		n, err := getLong(col, colGetCount)
		if err != nil {
			return err
		}
		for i := int32(1); i <= n; i++ {
			item, err := getItemLong(col, i)
			if err != nil {
				return err
			}
			typ, err := getLong(item, actionGetType)
			if err != nil {
				item.release()
				return err
			}
			a := action{Type: typ}
			if typ == actionExec {
				exec, qerr := item.query(&iidIExecAction)
				if qerr != nil {
					item.release()
					return qerr
				}
				a.Path, err = getBSTR(exec, execGetPath)
				if err == nil {
					a.Args, err = getBSTR(exec, execGetArguments)
				}
				exec.release()
			}
			item.release()
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return nil
	})
	return out, err
}

func (r *sysReg) triggers() ([]trigger, error) {
	var out []trigger
	err := withTask(r.folder, r.name, func(t, _ *comObj) error {
		def, err := definition(t)
		if err != nil {
			return err
		}
		defer def.release()
		col, err := getObj(def, defGetTriggers)
		if err != nil {
			return err
		}
		defer col.release()
		n, err := getLong(col, colGetCount)
		if err != nil {
			return err
		}
		for i := int32(1); i <= n; i++ {
			item, err := getItemLong(col, i)
			if err != nil {
				return err
			}
			typ, err := getLong(item, trigGetType)
			if err != nil {
				item.release()
				return err
			}
			var en int16
			if hr := item.call(trigGetEnabled, ptr(&en)); hr.failed() {
				item.release()
				return hr
			}
			tr := trigger{Type: typ, Enabled: en != 0}
			rep, err := getObj(item, trigGetRepetition)
			if err == nil && rep != nil {
				interval, err := getBSTR(rep, repGetInterval)
				rep.release()
				if err != nil {
					item.release()
					return err
				}
				tr.Repeats = interval != ""
			}
			item.release()
			out = append(out, tr)
		}
		return nil
	})
	return out, err
}

func (r *sysReg) setPath(path string) error {
	return withTask(r.folder, r.name, func(t, folder *comObj) error {
		def, err := definition(t)
		if err != nil {
			return err
		}
		defer def.release()
		prin, err := getObj(def, defGetPrincipal)
		if err != nil {
			return err
		}
		logon, err := getLong(prin, prinGetLogonType)
		prin.release()
		if err != nil {
			return err
		}
		col, err := getObj(def, defGetActions)
		if err != nil {
			return err
		}
		defer col.release()
		n, err := getLong(col, colGetCount)
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("selfupdate: taskscheduler: expected one action, have %d", n)
		}
		item, err := getItemLong(col, 1)
		if err != nil {
			return err
		}
		defer item.release()
		exec, err := item.query(&iidIExecAction)
		if err != nil {
			return err
		}
		defer exec.release()
		b, err := sysString(path)
		if err != nil {
			return err
		}
		defer freeBSTR(b)
		if hr := exec.call(execPutPath, b); hr.failed() {
			return hr
		}
		name, err := sysString(r.name)
		if err != nil {
			return err
		}
		defer freeBSTR(name)
		var empty [3]variant
		var out *comObj
		hr := folder.call(folderRegisterTaskDefinition,
			name,
			ptr(def),
			uintptr(taskUpdateFlags),
			ptr(&empty[0]),
			ptr(&empty[1]),
			uintptr(uint32(logon)), //nolint:gosec // TASK_LOGON_TYPE is a small enum
			ptr(&empty[2]),
			ptr(&out),
		)
		out.release()
		return checkHR(hr)
	})
}

func (r *sysReg) restore(xml string) error {
	return withFolder(r.folder, func(folder *comObj) error {
		name, err := sysString(r.name)
		if err != nil {
			return err
		}
		defer freeBSTR(name)
		body, err := sysString(xml)
		if err != nil {
			return err
		}
		defer freeBSTR(body)
		var empty [3]variant
		var out *comObj
		hr := folder.call(folderRegisterTask,
			name,
			body,
			uintptr(taskUpdateFlags),
			ptr(&empty[0]),
			ptr(&empty[1]),
			uintptr(logonInteractiveToken),
			ptr(&empty[2]),
			ptr(&out),
		)
		out.release()
		return checkHR(hr)
	})
}

func definition(task *comObj) (*comObj, error) {
	return getObj(task, taskGetDefinition)
}

func getObj(o *comObj, slot int) (*comObj, error) {
	var p *comObj
	if hr := o.call(slot, ptr(&p)); hr.failed() {
		return nil, hr
	}
	return p, nil
}

func getItemLong(col *comObj, i int32) (*comObj, error) {
	var p *comObj
	if hr := col.call(colGetItem, uintptr(uint32(i)), ptr(&p)); hr.failed() { //nolint:gosec // collection index is 1-based LONG
		return nil, hr
	}
	return p, nil
}
