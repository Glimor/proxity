//go:build windows

package main

import (
	"log/slog"
	"sync"

	"github.com/Glimor/proxity/internal/killswitch"
)

type lazyKillSwitchPolicy interface {
	Reconcile(killswitch.State) error
	SetHealthy(bool) error
	Close()
}

type lazyKillSwitch struct {
	mu      sync.Mutex
	p       lazyKillSwitchPolicy
	state   *killswitch.State
	healthy *bool
	closed  bool
	logger  *slog.Logger
}

func newLazyKillSwitch(logger *slog.Logger) *lazyKillSwitch {
	if logger == nil {
		logger = slog.Default()
	}
	return &lazyKillSwitch{logger: logger}
}

func (l *lazyKillSwitch) Reconcile(state killswitch.State) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}

	state.Apps = append([]killswitch.AppPolicy(nil), state.Apps...)
	l.state = &state
	if l.p == nil {
		return nil
	}
	return l.p.Reconcile(state)
}

func (l *lazyKillSwitch) SetHealthy(healthy bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}

	healthyCopy := healthy
	l.healthy = &healthyCopy
	if l.p == nil {
		return nil
	}
	return l.p.SetHealthy(healthy)
}

func (l *lazyKillSwitch) attach(p lazyKillSwitchPolicy) bool {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		p.Close()
		return false
	}

	l.p = p
	if l.state != nil {
		if err := p.Reconcile(*l.state); err != nil {
			l.logger.Error("kill switch state replay failed", slog.String("err", err.Error()))
		}
	}
	if l.healthy != nil {
		if err := p.SetHealthy(*l.healthy); err != nil {
			l.logger.Error("kill switch health replay failed", slog.String("err", err.Error()))
		}
	}
	l.mu.Unlock()
	return true
}

func (l *lazyKillSwitch) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	if l.p != nil {
		l.p.Close()
		l.p = nil
	}
}
