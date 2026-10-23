package taskscheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for DiffDefinitions (0032-MADR D3-A,
// 0032-PLAN-task-scheduler-registration-and-one-shot-handoff.md J4),
// against the definitions Task Scheduler stored in Amendment A1, P1-7.

var fixtureCases = []string{"comhandler", "consumer", "defaults", "minimal", "settings", "triggers"}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "definitions", name+".xml")) //nolint:gosec // a fixture
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustDiff(t *testing.T, stored, wanted string) []DefinitionDiff {
	t.Helper()
	diffs, err := DiffDefinitions(stored, wanted)
	if err != nil {
		t.Fatal(err)
	}
	return diffs
}

// TestDiffFixturesSame: each definition equals what Task Scheduler stored
// for it.
func TestDiffFixturesSame(t *testing.T) {
	for _, c := range fixtureCases {
		stored, wanted := fixture(t, c+".stored"), fixture(t, c+".wanted")
		if d := mustDiff(t, stored, wanted); len(d) != 0 {
			t.Errorf("%s: %+v", c, d)
		}
		if same, err := SameDefinition(stored, wanted); err != nil || !same {
			t.Errorf("%s: SameDefinition = %t, %v", c, same, err)
		}
	}
}

// TestDiffOneChange: changing one compared element of a wanted definition
// gives exactly one difference, at that element's path.
func TestDiffOneChange(t *testing.T) {
	for _, c := range []struct{ fixture, old, new, path string }{
		{"settings", "<AllowStartOnDemand>false", "<AllowStartOnDemand>true", "Settings/AllowStartOnDemand"},
		{"settings", "<MultipleInstancesPolicy>Parallel", "<MultipleInstancesPolicy>Queue", "Settings/MultipleInstancesPolicy"},
		{"settings", "<DisallowStartIfOnBatteries>false", "<DisallowStartIfOnBatteries>true", "Settings/DisallowStartIfOnBatteries"},
		{"settings", "<StopIfGoingOnBatteries>false", "<StopIfGoingOnBatteries>true", "Settings/StopIfGoingOnBatteries"},
		{"settings", "<AllowHardTerminate>false", "<AllowHardTerminate>true", "Settings/AllowHardTerminate"},
		{"settings", "<StartWhenAvailable>true", "<StartWhenAvailable>false", "Settings/StartWhenAvailable"},
		{"settings", "<RunOnlyIfNetworkAvailable>true", "<RunOnlyIfNetworkAvailable>false", "Settings/RunOnlyIfNetworkAvailable"},
		{"settings", "<Settings>\n  <AllowStartOnDemand>false</AllowStartOnDemand>", "<Settings>\n  <AllowStartOnDemand>false</AllowStartOnDemand><Volatile>true</Volatile>", "Settings/Volatile"},
		{"settings", "<Enabled>false</Enabled>\n  <Hidden>", "<Enabled>true</Enabled>\n  <Hidden>", "Settings/Enabled"},
		{"settings", "<Hidden>true", "<Hidden>false", "Settings/Hidden"},
		{"settings", "<RunOnlyIfIdle>true", "<RunOnlyIfIdle>false", "Settings/RunOnlyIfIdle"},
		{"settings", "<DisallowStartOnRemoteAppSession>true", "<DisallowStartOnRemoteAppSession>false", "Settings/DisallowStartOnRemoteAppSession"},
		{"settings", "<WakeToRun>true", "<WakeToRun>false", "Settings/WakeToRun"},
		{"settings", "<ExecutionTimeLimit>PT2H", "<ExecutionTimeLimit>PT3H", "Settings/ExecutionTimeLimit"},
		{"settings", "<DeleteExpiredTaskAfter>PT1H", "<DeleteExpiredTaskAfter>PT2H", "Settings/DeleteExpiredTaskAfter"},
		{"settings", "<Priority>5", "<Priority>6", "Settings/Priority"},
		{"settings", "<Interval>PT2M</Interval><Count>4", "<Interval>PT3M</Interval><Count>4", "Settings/RestartOnFailure/Interval"},
		{"settings", "<Count>4", "<Count>5", "Settings/RestartOnFailure/Count"},
		{"triggers", "<Delay>PT30S", "<Delay>PT31S", "Triggers/LogonTrigger[1]/Delay"},
		{"triggers", "<ExecutionTimeLimit>PT1H", "<ExecutionTimeLimit>PT2H", "Triggers/LogonTrigger[1]/ExecutionTimeLimit"},
		{"triggers", "<Interval>PT5M", "<Interval>PT6M", "Triggers/LogonTrigger[1]/Repetition/Interval"},
		{"triggers", "<Duration>PT1H", "<Duration>PT2H", "Triggers/LogonTrigger[1]/Repetition/Duration"},
		{"triggers", "<StopAtDurationEnd>true", "<StopAtDurationEnd>false", "Triggers/LogonTrigger[1]/Repetition/StopAtDurationEnd"},
		{"triggers", "<Enabled>false</Enabled><RandomDelay>", "<Enabled>true</Enabled><RandomDelay>", "Triggers/TimeTrigger[2]/Enabled"},
		{"triggers", "<RandomDelay>PT1M", "<RandomDelay>PT2M", "Triggers/TimeTrigger[2]/RandomDelay"},
		{"triggers", "<DaysInterval>2", "<DaysInterval>3", "Triggers/CalendarTrigger[3]/ScheduleByDay/DaysInterval"},
		{"triggers", "<WeeksInterval>1", "<WeeksInterval>2", "Triggers/CalendarTrigger[4]/ScheduleByWeek/WeeksInterval"},
		{"triggers", "<Delay>PT1M</Delay></RegistrationTrigger>", "<Delay>PT2M</Delay></RegistrationTrigger>", "Triggers/RegistrationTrigger[5]/Delay"},
		{"triggers", "<IdleTrigger><Enabled>false", "<IdleTrigger><Enabled>true", "Triggers/IdleTrigger[6]/Enabled"},
		{"triggers", "SessionUnlock", "SessionLock", "Triggers/SessionStateChangeTrigger[7]/StateChange"},
		{"triggers", "EventID=4242", "EventID=4243", "Triggers/EventTrigger[8]/Subscription"},
		{"consumer", "<StartBoundary>2000-01-01T00:00:00", "<StartBoundary>2001-01-01T00:00:00", "Triggers/TimeTrigger[2]/StartBoundary"},
		{"consumer", `<UserId>EXAMPLE\user</UserId></LogonTrigger>`, `<UserId>EXAMPLE\someone</UserId></LogonTrigger>`, "Triggers/LogonTrigger[1]/UserId"},
		{"consumer", "<RunLevel>LeastPrivilege", "<RunLevel>HighestAvailable", principalPath + "RunLevel"},
		{"consumer", "<LogonType>InteractiveToken", "<LogonType>Password", principalPath + "LogonType"},
		{"consumer", "-1001</UserId>", "-1002</UserId>", principalPath + "UserId"},
		{"consumer", "<Command>C:", "<Command>D:", "Actions/Exec[1]/Command"},
		{"consumer", "/t 5 /nobreak", "/t 6 /nobreak", "Actions/Exec[1]/Arguments"},
		{"consumer", `<WorkingDirectory>C:\Windows\Temp`, `<WorkingDirectory>C:\Temp`, "Actions/Exec[1]/WorkingDirectory"},
		{"comhandler", "{6B7C3F1A", "{6B7C3F1B", "Actions/ComHandler[1]/ClassId"},
		{"comhandler", "probe-data", "probe-other", "Actions/ComHandler[1]/Data"},
	} {
		wanted := fixture(t, c.fixture+".wanted")
		if strings.Count(wanted, c.old) != 1 {
			t.Fatalf("%s: %q occurs %d times", c.path, c.old, strings.Count(wanted, c.old))
		}
		d := mustDiff(t, fixture(t, c.fixture+".stored"), strings.Replace(wanted, c.old, c.new, 1))
		if len(d) != 1 || d[0].Path != c.path {
			t.Errorf("%s: diffs %+v", c.path, d)
		}
	}
}

// TestDiffDefaults: an element absent on one side compares as its
// default; set to another value, it differs.
func TestDiffDefaults(t *testing.T) {
	if d := mustDiff(t, fixture(t, "defaults.stored"), fixture(t, "minimal.wanted")); len(d) != 2 {
		// The two differ only in their action's arguments and directory.
		t.Fatalf("defaults stored against minimal wanted: %+v", d)
	}
	d := mustDiff(t, fixture(t, "settings.stored"), fixture(t, "defaults.wanted"))
	got := map[string]DefinitionDiff{}
	for _, x := range d {
		got[x.Path] = x
	}
	for _, p := range []string{"Settings/Priority", "Settings/Hidden", "Settings/ExecutionTimeLimit", "Settings/AllowStartOnDemand"} {
		if _, ok := got[p]; !ok {
			t.Errorf("%s not reported: %+v", p, d)
		}
	}
	if x := got["Settings/Priority"]; x.Stored != "5" || x.Wanted != "7" {
		t.Errorf("Priority diff %+v, want stored 5, wanted 7", x)
	}
	if x := got["Settings/RestartOnFailure/Count"]; x.Stored != "4" || x.Wanted != "" {
		t.Errorf("RestartOnFailure/Count diff %+v, want stored 4 and an absent wanted", x)
	}
}

// TestDiffIgnores: what Task Scheduler adds or owns is not compared.
func TestDiffIgnores(t *testing.T) {
	stored, wanted := fixture(t, "consumer.stored"), fixture(t, "consumer.wanted")
	for _, c := range []struct{ old, new string }{
		{"demo background service", "another description"},
		{`<Task version="1.4"`, `<Task version="1.2"`},
		{`<Principal id="Author">`, `<Principal id="Other">`},
	} {
		if d := mustDiff(t, stored, strings.Replace(wanted, c.old, c.new, 1)); len(d) != 0 {
			t.Errorf("%q: %+v", c.new, d)
		}
	}
	for _, c := range []struct{ old, new string }{
		{"<RestartOnIdle>false", "<RestartOnIdle>true"},
		{"<UseUnifiedSchedulingEngine>true", "<UseUnifiedSchedulingEngine>false"},
		{`<URI>\selfupdate-probe-00000000-p1-7-consumer`, `<URI>\elsewhere`},
	} {
		if d := mustDiff(t, strings.Replace(stored, c.old, c.new, 1), wanted); len(d) != 0 {
			t.Errorf("stored %q: %+v", c.new, d)
		}
	}
	if d := mustDiff(t, fixture(t, "minimal.stored"), fixture(t, "minimal.wanted")); len(d) != 0 {
		t.Fatalf("a wanted principal with no UserId: %+v", d)
	}
}

// TestDiffValues: durations and booleans compare as values.
func TestDiffValues(t *testing.T) {
	stored := fixture(t, "settings.stored")
	for _, c := range []struct{ old, new string }{
		{"<ExecutionTimeLimit>PT2H", "<ExecutionTimeLimit>PT120M"},
		{"<ExecutionTimeLimit>PT2H", "<ExecutionTimeLimit>PT7200S"},
		{"<Hidden>true", "<Hidden>1"},
		{"<Enabled>false</Enabled>\n    <ExecutionTimeLimit>", "<Enabled>0</Enabled>\n    <ExecutionTimeLimit>"},
	} {
		if strings.Count(stored, c.old) != 1 {
			t.Fatalf("%q occurs %d times", c.old, strings.Count(stored, c.old))
		}
		if d := mustDiff(t, strings.Replace(stored, c.old, c.new, 1), fixture(t, "settings.wanted")); len(d) != 0 {
			t.Errorf("%q: %+v", c.new, d)
		}
	}
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{"PT1M", "PT60S", true}, {"P1D", "PT24H", true}, {"P1W", "P7D", true},
		{"P1M", "P30D", false}, {"PT0S", "PT0.5S", false}, {"PT1H", "PT1H1S", false},
	} {
		if got := sameValue("Settings/ExecutionTimeLimit", c.a, c.b); got != c.same {
			t.Errorf("%s vs %s: %t, want %t", c.a, c.b, got, c.same)
		}
	}
	if !sameValue(principalPath+"UserId", `EXAMPLE\User`, `example\user`) {
		t.Error("accounts compare case-sensitively")
	}
}

func TestDiffErrors(t *testing.T) {
	good := fixture(t, "minimal.wanted")
	for _, bad := range []string{"<Task><Settings>", "<Task></Settings>", "not xml <"} {
		if _, err := DiffDefinitions(bad, good); err == nil {
			t.Errorf("stored %q: no error", bad)
		}
		if _, err := DiffDefinitions(good, bad); err == nil {
			t.Errorf("wanted %q: no error", bad)
		}
	}
}

func TestDefinitionDiffJSON(t *testing.T) {
	b, err := json.Marshal(DefinitionDiff{Path: "Settings/Priority", Stored: "5", Wanted: "7"})
	if want := `{"path":"Settings/Priority","stored":"5","wanted":"7"}`; err != nil || string(b) != want {
		t.Fatalf("%s, %v; want %s", b, err, want)
	}
}
