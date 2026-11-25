package main

import (
	"testing"

	"golang.org/x/sys/windows/svc/mgr"
)

func TestServiceRegistrationRefreshPreservesStartupPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    uint32
		delayed bool
	}{
		{name: "automatic", mode: mgr.StartAutomatic},
		{name: "automatic delayed", mode: mgr.StartAutomatic, delayed: true},
		{name: "manual", mode: mgr.StartManual},
		{name: "disabled", mode: mgr.StartDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := mgr.Config{
				DisplayName:      "Previous service name",
				StartType:        tc.mode,
				DelayedAutoStart: tc.delayed,
			}

			got := serviceRegistration(current)

			if got.StartType != tc.mode || got.DelayedAutoStart != tc.delayed {
				t.Fatalf("startup policy changed on refresh: start type = %d, delayed = %v; want %d, %v",
					got.StartType, got.DelayedAutoStart, tc.mode, tc.delayed)
			}
			if got.DisplayName != serviceDisplay {
				t.Fatalf("service metadata not refreshed: display name = %q", got.DisplayName)
			}
		})
	}
}
