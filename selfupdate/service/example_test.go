package service_test

import (
	"context"
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// PollHealthy waits for a lifecycle's health probe to report the service
// ready. A real probe reads the service manager; this one is ready at once,
// and a negative Settle skips the window that watches for a crash after
// the first ready answer (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// D14).
func ExamplePollHealthy() {
	probe := func(context.Context) (service.Health, error) {
		return service.Health{Ready: true, Instance: "42"}, nil
	}
	if err := service.PollHealthy(context.Background(), probe, service.PollOptions{Settle: -1}); err != nil {
		fmt.Println("not healthy:", err)
		return
	}
	fmt.Println("healthy")
	// Output: healthy
}
