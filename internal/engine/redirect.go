//go:build windows

package engine

import (
	"context"
	"log/slog"
	"net/netip"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/logging"
	"github.com/Glimor/proxity/internal/netpkt"
	"github.com/Glimor/proxity/internal/windivert"
)

// captureFilter drops loopback (incl. our 127/8 listener path) and keeps only
// outbound TCP. Injected inbound packets are never recaptured.
const captureFilter = "outbound and tcp and !loopback"

// natDst returns the (dstIP, dstPort) a flow's packets are rewritten to so the
// transparent listener can recover the natID.
func (e *Engine) natDst(f *flow) (netip.Addr, uint16) {
	if !f.ipv6 {
		return netpkt.EncodeNATIPv4(f.natID), ProxyPort
	}
	if e.v6Pool {
		return netip.IPv6Loopback(), uint16(poolBasePort + f.natID - 1)
	}
	return netpkt.EncodeNATIPv6(f.natID), ProxyPort
}

// replyNATID decodes the natID of a packet sent by our listener (identified by
// srcPort == ProxyPort or, in pool mode, src == ::1:poolPort).
func (e *Engine) replyNATID(p *netpkt.Packet) (uint32, bool) {
	if e.v6Pool && p.SrcIP == netip.IPv6Loopback() &&
		p.SrcPort >= poolBasePort && p.SrcPort < poolBasePort+uint16(e.poolSz) {
		return uint32(p.SrcPort-poolBasePort) + 1, true
	}
	if p.SrcPort != ProxyPort {
		return 0, false
	}
	if id, ok := netpkt.DecodeNATIPv4(p.SrcIP); ok {
		return id, true
	}
	return netpkt.DecodeNATIPv6(p.SrcIP)
}

func (e *Engine) captureLoop(ctx context.Context) {
	buf := make([]byte, 64*1024)
	var addr windivert.Address
	for {
		n, err := e.h.Recv(buf, &addr)
		if err != nil {
			if ctx.Err() == nil {
				e.log.Error("windivert recv failed", slog.String("err", err.Error()))
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
	// Listener replies: src is a NAT address carrying the natID.
	if id, ok := e.replyNATID(p); ok {
		f := e.flows.getByID(id)
		if f == nil {
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
// src=D:dport dst=NATIP:ProxyPort and injects it inbound so it reaches the
// transparent listener as a connection "from" the original destination.
func (e *Engine) rewriteToListener(p *netpkt.Packet, f *flow, addr *windivert.Address) {
	natIP, natPort := e.natDst(f)
	p.SetSrc(f.origDst, f.origDstPort)
	p.SetDst(natIP, natPort)
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

func (e *Engine) dropBlocked(p *netpkt.Packet, pid int, exeFold, reason string) {
	dst := p.DstIP.String()
	logging.Blocked(e.log, exeFold, pid, dst, reason)
	e.agg.recordBlocked(domain.BlockedEntry{
		TS: time.Now(), ExePath: exeFold, PID: pid,
		Dst: dst, Port: int(p.DstPort), Proto: "tcp", Reason: reason,
	})
	// Deliberate drop: the packet is never re-sent.
}
