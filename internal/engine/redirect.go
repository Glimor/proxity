//go:build windows

package engine

import (
	"context"
	"log/slog"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/logging"
	"github.com/Glimor/proxity/internal/netpkt"
	"github.com/Glimor/proxity/internal/windivert"
)

// captureFilter drops loopback (incl. our 127/8 listener path) and keeps only
// outbound TCP. Injected inbound packets are never recaptured.
const captureFilter = "outbound and tcp and !loopback"

func (e *Engine) captureLoop(ctx context.Context) {
	buf := make([]byte, 64*1024)
	var addr windivert.Address
	for {
		n, err := e.h.Recv(buf, &addr)
		if err != nil {
			if ctx.Err() == nil {
				e.log.Error("windivert recv failed", slog.String("err", err.Error()))
				// The capture loop is dead: nothing is being intercepted
				// any more, so the kill switch must fail closed.
				e.captureDead()
			}
			return
		}
		raw := buf[:n]
		p, ok := netpkt.Parse(raw)
		if !ok {
			e.send(raw, &addr)
			continue
		}
		e.handlePacket(p, &addr)
	}
}

func (e *Engine) handlePacket(p *netpkt.Packet, addr *windivert.Address) {
	// Listener replies: srcPort == ProxyPort, dstPort carries the natID.
	// Our listeners own ProxyPort on both stacks, so a real app flow can
	// never source from it — check this before the flow-table lookup.
	if p.SrcPort == ProxyPort {
		f := e.flows.getByID(uint32(p.DstPort))
		if f == nil || f.origDst != p.DstIP {
			e.send(p.Raw, addr)
			return
		}
		p.SetSrc(f.origDst, f.origDstPort)
		p.SetDst(f.origSrc, f.origSrcPort)
		p.FixChecksums()
		e.inject(p, addr)
		return
	}

	k := tupleKey{p.SrcIP, p.DstIP, p.SrcPort, p.DstPort}
	if f := e.flows.getByTuple(k); f != nil {
		e.flows.touch(f)
		if f.action != domain.DecisionProxy {
			e.send(p.Raw, addr)
			return
		}
		e.rewriteToListener(p, f, addr)
		return
	}
	if !p.IsSYN() {
		e.send(p.Raw, addr)
		return
	}
	e.decide(p, k, addr)
}

// rewriteToListener turns an app-side packet src=A:sport dst=D:dport into
// src=D:natID dst=A:ProxyPort and injects it inbound so it reaches the
// transparent listener as a connection "from" the original destination. The
// natID rides in the reflected source port; the listener recovers everything
// else from the flow table. Since dst is the app's own real source address —
// always a configured local address — delivery works on both IP families.
func (e *Engine) rewriteToListener(p *netpkt.Packet, f *flow, addr *windivert.Address) {
	p.SetSrc(f.origDst, uint16(f.natID))
	p.SetDst(f.origSrc, ProxyPort)
	p.FixChecksums()
	e.inject(p, addr)
}

func (e *Engine) inject(p *netpkt.Packet, addr *windivert.Address) {
	addr.SetOutbound(false)
	if _, err := e.h.Send(p.Raw, addr); err != nil {
		e.log.Debug("inject failed", slog.String("err", err.Error()))
	}
}

func (e *Engine) send(raw []byte, addr *windivert.Address) {
	if _, err := e.h.Send(raw, addr); err != nil {
		e.log.Debug("send failed", slog.String("err", err.Error()))
	}
}

// decide handles a connection-opening SYN with no flow entry.
func (e *Engine) decide(p *netpkt.Packet, k tupleKey, addr *windivert.Address) {
	cfg := e.config()
	pid, exeFold, _ := e.pm.Lookup(p.SrcIP, p.SrcPort)

	pass := func() {
		f := &flow{origSrc: p.SrcIP, origSrcPort: p.SrcPort, origDst: p.DstIP,
			origDstPort: p.DstPort, ipv6: p.IPv6, pid: pid, exeFold: exeFold,
			action: domain.DecisionDirect}
		e.flows.add(f, false)
		e.send(p.Raw, addr)
	}

	switch {
	case pid != 0 && pid == e.selfPID:
		pass()
	case cfg.isProxyEndpoint(p.DstIP, p.DstPort):
		pass()
	default:
		rule := domain.MatchRule(cfg.rules, exeFold)
		if rule == nil {
			e.send(p.Raw, addr)
			return
		}
		switch rule.Action {
		case domain.RuleActionDirect:
			pass()
		case domain.RuleActionBlock:
			e.dropBlocked(p, pid, exeFold, "rule:"+rule.Name)
		case domain.RuleActionProxy:
			prof, ok := cfg.profiles[rule.ProxyID]
			healthy := ok && prof.Enabled && e.hc.healthy(prof.ID)
			if !healthy {
				if rule.KillSwitch {
					e.dropBlocked(p, pid, exeFold, "kill-switch:"+rule.Name)
					return
				}
				pass()
				return
			}
			f := &flow{origSrc: p.SrcIP, origSrcPort: p.SrcPort, origDst: p.DstIP,
				origDstPort: p.DstPort, ipv6: p.IPv6, pid: pid, exeFold: exeFold,
				action: domain.DecisionProxy, ruleID: rule.ID, proxyID: prof.ID}
			f.connID = e.agg.register(domain.Connection{
				PID: pid, ExePath: exeFold, DstIP: p.DstIP.String(),
				DstPort: int(p.DstPort), IPv6: p.IPv6, Action: domain.DecisionProxy,
				ProxyID: prof.ID, MatchedRuleID: rule.ID,
			})
			if !e.flows.add(f, true) {
				e.agg.close(f.connID, domain.ConnFailed)
				e.dropBlocked(p, pid, exeFold, "nat-id-exhausted")
				return
			}
			e.rewriteToListener(p, f, addr)
		default:
			e.send(p.Raw, addr)
		}
	}
}

// blockedDedupWindow suppresses repeat blocked-log entries for the same
// 4-tuple while a stubborn app keeps retransmitting its SYN.
const blockedDedupWindow = 3 * time.Second

func (e *Engine) dropBlocked(p *netpkt.Packet, pid int, exeFold, reason string) {
	k := tupleKey{p.SrcIP, p.DstIP, p.SrcPort, p.DstPort}
	now := time.Now()
	e.blockedMu.Lock()
	if t, ok := e.blockedAt[k]; ok && now.Sub(t) < blockedDedupWindow {
		e.blockedMu.Unlock()
		return
	}
	e.blockedAt[k] = now
	if len(e.blockedAt) > 1024 { // opportunistic prune
		for kk, t := range e.blockedAt {
			if now.Sub(t) >= blockedDedupWindow {
				delete(e.blockedAt, kk)
			}
		}
	}
	e.blockedMu.Unlock()

	dst := p.DstIP.String()
	logging.Blocked(e.log, exeFold, pid, dst, reason)
	e.agg.recordBlocked(domain.BlockedEntry{
		TS: time.Now(), ExePath: exeFold, PID: pid,
		Dst: dst, Port: int(p.DstPort), Proto: "tcp", Reason: reason,
	})
	// Deliberate drop: the packet is never re-sent.
}
