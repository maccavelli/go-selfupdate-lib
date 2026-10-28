package servicetest_test

import (
	"context"
	"fmt"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// A program checks its own Lifecycle the same way, with its own Lab. Here
// the reference FakeService is both, with short bounds, so the example
// runs on the real clock in about a second; in a test, run it in a
// testing/synctest bubble with the bounds the lifecycle documents.
func ExampleVerifyLifecycle() {
	f := servicetest.NewFakeService(servicetest.FakeOptions{
		StopBound: 200 * time.Millisecond, StartBound: 200 * time.Millisecond,
		StopLatency: 50 * time.Millisecond, Interval: 10 * time.Millisecond,
	})
	err := servicetest.VerifyLifecycle(context.Background(), f, servicetest.LifecycleCase{
		Product: "demo", Lab: f,
		StopBound: f.StopBound(), StartBound: f.StartBound(), Slack: 100 * time.Millisecond,
		RestoresAfterFailedStop: true,
	})
	fmt.Println("violations:", err)
	// Output: violations: <nil>
}
