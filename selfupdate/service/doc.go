// Package service holds what the reference service lifecycles share
// (docs/decisions/0011-MADR-reference-service-lifecycles.md): a command
// runner, the typed errors, PollHealthy, ExecReconciler, and the handoff
// that keeps an updater from stopping the service it runs inside.
//
// The lifecycles themselves are in the subpackages systemd, launchd and
// scm. Each implements selfupdate.Lifecycle, selfupdate.EnabledLifecycle,
// selfupdate.Reconciler and Detacher, compiles on every OS, and returns
// ErrUnsupported on the wrong one.
//
// # The handoff
//
// An update run from inside the service it manages, such as by an agent the
// service spawned, would die when it stopped the service, and leave it
// stopped. Before anything changes, HandOffIfInside asks the Detacher
// whether this process is inside the service's kill scope; when it is, the
// same command is started again outside that scope, with SELFUPDATE_HANDOFF
// and SELFUPDATE_HANDOFF_RESULT set, and the caller returns at once. The
// detached run performs the whole managed update and writes a
// HandOffResult, which ReadHandOffResult reads after the caller reconnects.
// cli.Options.HandOff takes HandOffFunc and ReportFunc.
package service
