package taskscheduler

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// TestNewOffWindows: New refuses off Windows, and without a transport.
func TestNewOffWindows(t *testing.T) {
	if _, err := newTask(Options{}, "linux", newFake()); !errors.Is(err, service.ErrUnsupported) {
		t.Fatalf("linux: %v, want ErrUnsupported", err)
	}
	if _, err := newTask(Options{}, "windows", nil); !errors.Is(err, service.ErrUnsupported) {
		t.Fatalf("no transport: %v, want ErrUnsupported", err)
	}
}

// TestOptionsValidated: names, folders, StopGrace and Poll are checked
// once, at New.
func TestOptionsValidated(t *testing.T) {
	for _, c := range []struct {
		name string
		o    Options
		ok   bool
	}{
		{"defaults", Options{}, true},
		{"name", Options{Name: "demo"}, true},
		{"name with backslash", Options{Name: `a\b`}, false},
		{"name with slash", Options{Name: "a/b"}, false},
		{"root folder", Options{Folder: `\`}, true},
		{"nested folder", Options{Folder: `\Vendor\Product`}, true},
		{"folder without root", Options{Folder: `Vendor`}, false},
		{"folder with trailing separator", Options{Folder: `\Vendor\`}, false},
		{"folder with empty element", Options{Folder: `\Vendor\\Product`}, false},
		{"folder with slash", Options{Folder: `\Vendor/Product`}, false},
		{"negative grace", Options{StopGrace: -time.Second}, false},
		{"settle not under timeout", Options{Poll: service.PollOptions{Settle: time.Minute, Timeout: time.Second}}, false},
	} {
		_, err := newTask(c.o, "windows", newFake())
		if (err == nil) != c.ok {
			t.Errorf("%s: %v, want ok %t", c.name, err, c.ok)
		}
	}
	task, err := newTask(Options{}, "windows", newFake())
	if err != nil {
		t.Fatal(err)
	}
	if task.o.Folder != `\` || task.o.StopGrace != DefaultStopGrace {
		t.Fatalf("defaults: Folder %q, StopGrace %s", task.o.Folder, task.o.StopGrace)
	}
}

// TestTaskPath: the task's path joins its folder and its name, and the
// product names it when Options.Name is empty.
func TestTaskPath(t *testing.T) {
	for _, c := range []struct {
		o       Options
		product string
		want    string
	}{
		{Options{Name: "demo"}, "other", `\demo`},
		{Options{}, "demo", `\demo`},
		{Options{Name: "demo", Folder: `\Vendor`}, "", `\Vendor\demo`},
	} {
		task, err := newTask(c.o, "windows", newFake())
		if err != nil {
			t.Fatal(err)
		}
		if _, path, err := task.task(c.product); err != nil || path != c.want {
			t.Errorf("%+v %q: %q, %v; want %q", c.o, c.product, path, err, c.want)
		}
	}
	task, _ := newTask(Options{}, "windows", newFake())
	if _, _, err := task.task(`a\b`); err == nil {
		t.Fatal("a product holding a separator was accepted")
	}
}

// TestHRESULTMap: a missing task is ErrNotInstalled, a refusal
// ErrPermission, and any other HRESULT keeps its code, in hex (0021-MADR
// D2; Amendment A1, probe 5).
func TestHRESULTMap(t *testing.T) {
	if err := mapError("open", `\demo`, hrFileNotFound); !errors.Is(err, service.ErrNotInstalled) {
		t.Errorf("0x80070002: %v", err)
	}
	if err := mapError("open", `\demo`, hrAccessDenied); !errors.Is(err, service.ErrPermission) {
		t.Errorf("0x80070005: %v", err)
	}
	err := mapError("run", `\demo`, hresult(0x80041326))
	if errors.Is(err, service.ErrNotInstalled) || errors.Is(err, service.ErrPermission) || !strings.Contains(err.Error(), "HRESULT 0x80041326") {
		t.Errorf("other: %v", err)
	}
}
