// Package servicetest checks a selfupdate.Lifecycle, and a service.Runner,
// against the contract the managed installer relies on, as
// selfupdatetest.VerifyReleaseSource checks a release source and
// testing/fstest.TestFS a file system
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D2-A).
//
// VerifyLifecycle drives a Lifecycle through the states a Lab arranges:
// absent, stopped, running, slow to stop, ignoring a stop, exiting at once,
// and never finishing a start. It returns every violation it finds,
// joined, or nil. VerifyRunner does the same for a Runner's handling of
// its context. Neither takes a *testing.T, so they run from any harness.
//
// Over a fake, run VerifyLifecycle inside a testing/synctest bubble: the
// bounds a lifecycle documents, such as a 60-second stop, then cost no wall
// time, and the lifecycle's own clock is the bubble's. Over a real service
// manager, run it on the real clock with short bounds, and pass the bound
// the implementation documents for them as LifecycleCase.StopBound.
//
// FakeService is an in-memory service that is a Lifecycle, an
// EnabledLifecycle and its own Lab. It is the reference VerifyLifecycle's
// own tests pass, and a program can test its managed-update flow over it.
//
// The package imports only the standard library, selfupdate and service.
package servicetest
