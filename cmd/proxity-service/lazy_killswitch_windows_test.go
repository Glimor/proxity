//go:build windows

package main

import (
	"reflect"
	"testing"

	"github.com/Glimor/proxity/internal/killswitch"
)

type fakeLazyKillSwitchPolicy struct {
	calls   []string
	states  []killswitch.State
	healthy []bool
}

func (f *fakeLazyKillSwitchPolicy) Reconcile(state killswitch.State) error {
	f.calls = append(f.calls, "reconcile")
	f.states = append(f.states, state)
	return nil
}

func (f *fakeLazyKillSwitchPolicy) SetHealthy(healthy bool) error {
	f.calls = append(f.calls, "healthy")
	f.healthy = append(f.healthy, healthy)
	return nil
}

func (*fakeLazyKillSwitchPolicy) Close() {}

func TestLazyKillSwitchReplaysLatestStateAndHealth(t *testing.T) {
	lazy := newLazyKillSwitch(nil)

	if err := lazy.Reconcile(killswitch.State{Global: true}); err != nil {
		t.Fatal(err)
	}
	if err := lazy.SetHealthy(false); err != nil {
		t.Fatal(err)
	}
	wantState := killswitch.State{LANAllow: true}
	if err := lazy.Reconcile(wantState); err != nil {
		t.Fatal(err)
	}
	if err := lazy.SetHealthy(true); err != nil {
		t.Fatal(err)
	}

	policy := &fakeLazyKillSwitchPolicy{}
	if !lazy.attach(policy) {
		t.Fatal("attach returned false")
	}
	if !reflect.DeepEqual(policy.calls, []string{"reconcile", "healthy"}) {
		t.Fatalf("calls = %v, want [reconcile healthy]", policy.calls)
	}
	if len(policy.states) != 1 || !reflect.DeepEqual(policy.states[0], wantState) {
		t.Fatalf("replayed states = %#v, want %#v", policy.states, []killswitch.State{wantState})
	}
	if !reflect.DeepEqual(policy.healthy, []bool{true}) {
		t.Fatalf("replayed health = %v, want [true]", policy.healthy)
	}
}
