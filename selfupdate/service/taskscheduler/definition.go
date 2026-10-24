package taskscheduler

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// DefinitionDiff is one difference DiffDefinitions found: an element path,
// such as Settings/DisallowStartIfOnBatteries or
// Triggers/TimeTrigger[2]/Repetition/Interval, and its value on each side.
// A value is empty when the element is absent and has no default, and
// "(set)" for an element present with no text, such as a day of the week.
type DefinitionDiff struct {
	Path   string `json:"path"`
	Stored string `json:"stored"`
	Wanted string `json:"wanted"`
}

// SameDefinition reports whether DiffDefinitions finds no difference.
func SameDefinition(stored, wanted string) (bool, error) {
	diffs, err := DiffDefinitions(stored, wanted)
	return len(diffs) == 0, err
}

// DiffDefinitions compares a definition Task Scheduler stored, as
// Definition returns it, with the definition a program wants, and returns
// each difference, sorted by path (0032-MADR D3-A). Task Scheduler stores
// a definition in another form than it was registered in: it drops
// values that are defaults, writes others it adds, stores the principal's
// SID, and reorders (0032-MADR F13; Amendment A1, P1-7). So the comparison
// is of values, not text:
//
//   - compared: every trigger, in order, by type, with every element it
//     holds; the principal's UserId, GroupId, LogonType and RunLevel;
//     every Settings element except IdleSettings and
//     UseUnifiedSchedulingEngine, which Task Scheduler adds, and Enabled,
//     the task's switch, which Stop, Enable and Disable set and
//     Status.Disabled reads
//     (docs/decisions/0034-MADR-export-task-scheduler-enable-and-disable.md
//     D5-B); and every action, in order, with every element it holds;
//   - not compared: RegistrationInfo, the task's Data, attributes, and
//     anything else; a difference there is not reported;
//   - an element absent on one side compares as its default, where Task
//     Scheduler has one (the table in this file); a principal with no
//     UserId compares equal to any, because Task Scheduler registers it
//     for the registering user;
//   - durations compare as durations (PT1M equals PT60S), booleans as
//     booleans (1 equals true), accounts case-insensitively, and on
//     Windows a SID equals the account it names; other text is trimmed.
//
// The XML declaration of either side is ignored: the COM API hands over a
// decoded string. Malformed XML on either side is an error.
func DiffDefinitions(stored, wanted string) ([]DefinitionDiff, error) {
	s, err := flatten(stored)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: taskscheduler: the stored definition: %w", err)
	}
	w, err := flatten(wanted)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: taskscheduler: the wanted definition: %w", err)
	}
	paths := map[string]bool{}
	for p := range s {
		paths[p] = true
	}
	for p := range w {
		paths[p] = true
	}
	var out []DefinitionDiff
	for _, p := range slices.Sorted(maps.Keys(paths)) {
		if !compared(p) {
			continue
		}
		sv, sok := s[p]
		wv, wok := w[p]
		if p == principalPath+"UserId" && !wok {
			continue
		}
		if !sok {
			sv = defaultOf(p)
		}
		if !wok {
			wv = defaultOf(p)
		}
		if !sameValue(p, sv, wv) {
			out = append(out, DefinitionDiff{Path: p, Stored: sv, Wanted: wv})
		}
	}
	return out, nil
}

const principalPath = "Principals/Principal/"

// indexed are the containers whose children are numbered by position:
// Triggers/LogonTrigger[1], Actions/Exec[2].
var indexed = map[string]bool{"Triggers": true, "Actions": true}

// flatten reads a definition's leaf elements by path, relative to Task.
func flatten(def string) (map[string]string, error) {
	d := xml.NewDecoder(strings.NewReader(stripDeclaration(def)))
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	out := map[string]string{}
	type frame struct {
		name     string
		text     strings.Builder
		children int
	}
	var stack []*frame
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			name := tok.Name.Local
			if n := len(stack); n > 0 {
				parent := stack[n-1]
				parent.children++
				if indexed[parent.name] && n == 2 {
					name += "[" + strconv.Itoa(parent.children) + "]"
				}
			}
			stack = append(stack, &frame{name: name})
		case xml.CharData:
			if n := len(stack); n > 0 {
				stack[n-1].text.Write(tok)
			}
		case xml.EndElement:
			n := len(stack)
			if n == 0 {
				return nil, errors.New("unbalanced XML")
			}
			top := stack[n-1]
			stack = stack[:n-1]
			if top.children > 0 || n == 1 {
				continue
			}
			names := make([]string, 0, n-1)
			for _, f := range stack[1:] {
				names = append(names, f.name)
			}
			path := strings.Join(append(names, top.name), "/")
			v := strings.TrimSpace(top.text.String())
			if v == "" {
				v = "(set)"
			}
			out[path] = v
		}
	}
	if len(stack) != 0 {
		return nil, errors.New("unexpected end of XML")
	}
	return out, nil
}

// compared reports whether DiffDefinitions compares an element.
func compared(path string) bool {
	switch {
	case strings.HasPrefix(path, principalPath):
		switch strings.TrimPrefix(path, principalPath) {
		case "UserId", "GroupId", "LogonType", "RunLevel":
			return true
		}
		return false
	case strings.HasPrefix(path, "Settings/"):
		return !strings.HasPrefix(path, "Settings/IdleSettings/") && path != "Settings/UseUnifiedSchedulingEngine" && path != settingsSwitch
	case strings.HasPrefix(path, "Triggers/"), strings.HasPrefix(path, "Actions/"):
		return strings.Count(path, "/") >= 2
	}
	return false
}

// settingsDefaults are the values Task Scheduler gives a Settings element
// a definition leaves out, from the schema's settingsType
// (https://learn.microsoft.com/en-us/windows/win32/taskschd/taskschedulerschema-settingstype-complextype)
// and its element pages, and the stored XML 0032-MADR Amendment A1, P1-7
// captured: a definition with every setting at these values and one with
// none stored the same settings. StopIfGoingOnBatteries, whose page
// contradicts itself, is true as stored. Elements with no default here
// (RestartOnFailure, NetworkSettings, MaintenanceSettings) compare as
// absent.
var settingsDefaults = map[string]string{
	"AllowStartOnDemand":              xmlTrue,
	"AllowHardTerminate":              xmlTrue,
	"DeleteExpiredTaskAfter":          "PT0S",
	"DisallowStartIfOnBatteries":      xmlTrue,
	"DisallowStartOnRemoteAppSession": xmlFalse,
	"Enabled":                         xmlTrue,
	"ExecutionTimeLimit":              "PT72H",
	"Hidden":                          xmlFalse,
	"MultipleInstancesPolicy":         "IgnoreNew",
	"Priority":                        "7",
	"RunOnlyIfIdle":                   xmlFalse,
	"RunOnlyIfNetworkAvailable":       xmlFalse,
	"StartWhenAvailable":              xmlFalse,
	"StopIfGoingOnBatteries":          xmlTrue,
	"Volatile":                        xmlFalse,
	"WakeToRun":                       xmlFalse,
}

// triggerDefaults are a trigger's defaults, by the path below the
// trigger, from the schema's triggerBaseType and repetitionType, as the
// captures show: an Enabled true and a StopAtDurationEnd false are not
// stored.
var triggerDefaults = map[string]string{
	"Enabled":                      xmlTrue,
	"Repetition/StopAtDurationEnd": xmlFalse,
}

// principalDefaults are the principal's defaults [MS-TSCH], as the
// captures show.
var principalDefaults = map[string]string{
	"LogonType": "InteractiveToken",
	"RunLevel":  "LeastPrivilege",
}

// settingsSwitch is the task's switch, as its definition stores it: Task
// Scheduler writes put_Enabled into it (0034-MADR Amendment A1, P1), so
// it is state, not part of the definition a program renders, and
// DiffDefinitions does not compare it. settingsDefaults keeps its
// default.
const settingsSwitch = "Settings/Enabled"

// triggerPart matches Triggers/<Type>[n]/ and captures what follows.
var triggerPart = regexp.MustCompile(`^Triggers/[A-Za-z]+\[\d+\]/(.+)$`)

func defaultOf(path string) string {
	switch {
	case strings.HasPrefix(path, "Settings/"):
		return settingsDefaults[strings.TrimPrefix(path, "Settings/")]
	case strings.HasPrefix(path, principalPath):
		return principalDefaults[strings.TrimPrefix(path, principalPath)]
	}
	if m := triggerPart.FindStringSubmatch(path); m != nil {
		return triggerDefaults[m[1]]
	}
	return ""
}

// durationNames are the elements whose values are XML durations.
var durationNames = map[string]bool{
	"Delay": true, "RandomDelay": true, "ExecutionTimeLimit": true, "Interval": true,
	"Duration": true, "DeleteExpiredTaskAfter": true, "WaitTimeout": true,
	"Period": true, "Deadline": true,
}

func sameValue(path, a, b string) bool {
	if a == b {
		return true
	}
	last := path[strings.LastIndex(path, "/")+1:]
	if last == "UserId" {
		return a != "" && b != "" && sameAccount(a, b)
	}
	if durationNames[last] {
		da, aok := parseDuration(a)
		db, bok := parseDuration(b)
		if aok && bok {
			return da == db
		}
	}
	if ba, aok := parseBool(a); aok {
		if bb, bok := parseBool(b); bok {
			return ba == bb
		}
	}
	return false
}

// The schema's boolean spellings.
const (
	xmlTrue  = "true"
	xmlFalse = "false"
)

func parseBool(s string) (v, ok bool) {
	switch s {
	case xmlTrue, "1":
		return true, true
	case xmlFalse, "0":
		return false, true
	}
	return false, false
}

// duration is an XML duration's calendar part, in months, and its exact
// part, in milliseconds: P1M and P30D are not the same.
type duration struct{ months, millis int64 }

var durationRe = regexp.MustCompile(`^P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

func parseDuration(s string) (duration, bool) {
	m := durationRe.FindStringSubmatch(s)
	if m == nil || s == "P" || strings.HasSuffix(s, "T") {
		return duration{}, false
	}
	var n [7]int64
	for i := 1; i <= 6; i++ {
		if m[i] == "" {
			continue
		}
		v, err := strconv.ParseInt(m[i], 10, 64)
		if err != nil {
			return duration{}, false
		}
		n[i] = v
	}
	var secs float64
	if m[7] != "" {
		v, err := strconv.ParseFloat(m[7], 64)
		if err != nil {
			return duration{}, false
		}
		secs = v
	}
	return duration{
		months: n[1]*12 + n[2],
		millis: ((n[3]*7+n[4])*86400+n[5]*3600+n[6]*60)*1000 + int64(secs*1000),
	}, true
}
