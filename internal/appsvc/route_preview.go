//go:build windows

package appsvc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/Glimor/proxity/internal/domain"
)

type RouteMatchReason string

const (
	RouteDefault       RouteMatchReason = "default"
	RouteExactExe      RouteMatchReason = "exact_exe"
	RouteDeepestFolder RouteMatchReason = "deepest_folder"
)

type RouteProxyAvailability string

const (
	RouteProxyMissing     RouteProxyAvailability = "missing"
	RouteProxyDisabled    RouteProxyAvailability = "disabled"
	RouteProxyNotChecked  RouteProxyAvailability = "not_checked"
	RouteProxyConnected   RouteProxyAvailability = "connected"
	RouteProxyUnavailable RouteProxyAvailability = "unavailable"
)

type RouteExePolicy string

const (
	RouteExeNone       RouteExePolicy = "none"
	RouteExeBlock      RouteExePolicy = "block"
	RouteExeKillSwitch RouteExePolicy = "kill_switch"
	RouteExeUDPOnly    RouteExePolicy = "udp_only"
)

type RoutePreviewRule struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Kind     domain.RuleKind `json:"kind"`
	Priority *int            `json:"priority,omitempty"`
}

type RoutePreviewProxy struct {
	Name           string                 `json:"name,omitempty"`
	Availability   RouteProxyAvailability `json:"availability"`
	FallbackAction domain.RuleAction      `json:"fallback_action"`
}

// RoutePreview describes the saved rule branch for new remote TCP plus separate
// engine and firewall context. It is not a packet decision or a health test.
type RoutePreview struct {
	ExePath            string             `json:"exe_path"`
	Action             domain.RuleAction  `json:"action"`
	Reason             RouteMatchReason   `json:"reason"`
	Rule               *RoutePreviewRule  `json:"rule,omitempty"`
	Proxy              *RoutePreviewProxy `json:"proxy,omitempty"`
	Engine             EngineStatus       `json:"engine"`
	SavedDefaultAction domain.Decision    `json:"saved_default_action,omitempty"`
	ExePolicy          RouteExePolicy     `json:"exe_policy"`
	KillSwitchGlobal   bool               `json:"kill_switch_global"`
	LANAccess          bool               `json:"lan_access"`
	SecureDNS          bool               `json:"secure_dns"`
	ServiceExecutable  bool               `json:"service_executable"`
}

type routePreviewInput struct {
	ExePath string `json:"exe_path"`
}

func (s *Service) handleRoutePreview(ctx context.Context, params json.RawMessage) (any, error) {
	var in routePreviewInput
	if err := decode(params, &in); err != nil {
		return nil, domain.NewError("route.bad_request", "Invalid route preview request", "")
	}
	exe, err := parsePreviewExecutable(in.ExePath)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx(ctx)
	defer cancel()
	return s.previewRoute(ctx, exe)
}

type previewExecutable string

var reservedWindowsName = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[1-9¹²³]|CONIN\$|CONOUT\$)$`)

func parsePreviewExecutable(raw string) (previewExecutable, error) {
	badPath := domain.NewError("route.bad_exe_path", "Choose an executable with an absolute Windows .exe path", "")
	p := strings.TrimSpace(strings.ReplaceAll(raw, "/", `\`))
	if !filepath.IsAbs(p) || !strings.EqualFold(filepath.Ext(p), ".exe") ||
		strings.ContainsAny(p, `<>"|?*`) || strings.ContainsFunc(p, unicode.IsControl) {
		return "", badPath
	}
	vol := filepath.VolumeName(p)
	switch {
	case len(vol) == 2 && vol[1] == ':' && strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ", unicode.ToUpper(rune(vol[0]))):
	case strings.HasPrefix(vol, `\\`):
		host, share, ok := strings.Cut(vol[2:], `\`)
		if !ok || host == "" || host == "." || host == ".." || share == "" || share == "." || share == ".." ||
			strings.ContainsAny(vol, ":") {
			return "", badPath
		}
	default:
		return "", badPath
	}
	rest := p[len(vol):]
	if !strings.HasPrefix(rest, `\`) || strings.Contains(rest, ":") {
		return "", badPath
	}
	for _, part := range strings.Split(rest[1:], `\`) {
		stem, _, _ := strings.Cut(part, ".")
		if !filepath.IsLocal(part) || strings.TrimRight(part, ". ") != part ||
			reservedWindowsName.MatchString(strings.TrimRight(stem, " ")) {
			return "", badPath
		}
	}
	canonical, err := domain.CanonicalPath(p)
	if err != nil {
		return "", badPath
	}
	return previewExecutable(canonical), nil
}

func (s *Service) previewRoute(ctx context.Context, exe previewExecutable) (RoutePreview, error) {
	rules, err := s.ListRules(ctx)
	if err != nil {
		return RoutePreview{}, domain.NewError("route.rules_unavailable", "Could not read saved rules", "")
	}
	st, err := s.GetSettings(ctx)
	if err != nil {
		return RoutePreview{}, domain.NewError("route.settings_unavailable", "Could not read saved settings", "")
	}
	fold := domain.FoldPath(string(exe))
	policy := s.ksState(rules, st)
	out := RoutePreview{
		ExePath: string(exe), Action: domain.RuleActionDirect, Reason: RouteDefault,
		Engine: s.EngineStatus(), ExePolicy: RouteExeNone, KillSwitchGlobal: policy.Global,
		LANAccess: policy.LANAllow, SecureDNS: policy.SecureDNS,
		ServiceExecutable: fold == domain.FoldPath(policy.SelfPath),
	}
	for _, app := range policy.Apps {
		if app.PathFold != fold {
			continue
		}
		switch {
		case app.BlockOnly:
			out.ExePolicy = RouteExeBlock
		case app.UDPBlock:
			out.ExePolicy = RouteExeUDPOnly
		default:
			out.ExePolicy = RouteExeKillSwitch
		}
		break
	}
	winner := domain.MatchRule(rules, fold)
	if winner == nil {
		// The capture engine's no-match branch passes direct; it does not
		// consume Settings.DefaultAction.
		out.SavedDefaultAction = st.DefaultAction
		return out, nil
	}
	out.Action = winner.Action
	out.Reason = RouteDeepestFolder
	if winner.Kind == domain.RuleKindExe {
		out.Reason = RouteExactExe
	}
	out.Rule = &RoutePreviewRule{Name: winner.Name, Path: winner.Path, Kind: winner.Kind}
	priorityDistinguished := false
	for i := range rules {
		r := &rules[i]
		if r == winner || r.Kind != winner.Kind || len(r.PathFold) != len(winner.PathFold) || !r.Matches(fold) {
			continue
		}
		if r.Priority == winner.Priority {
			priorityDistinguished = false // saved order, not priority, broke the top tie
			break
		}
		priorityDistinguished = true
	}
	if priorityDistinguished {
		out.Rule.Priority = &winner.Priority
	}
	if winner.Action != domain.RuleActionProxy {
		return out, nil
	}
	profiles, err := s.ListProfiles(ctx)
	if err != nil {
		return RoutePreview{}, domain.NewError("route.profiles_unavailable", "Could not read saved profiles", "")
	}
	proxy := &RoutePreviewProxy{Availability: RouteProxyMissing, FallbackAction: domain.RuleActionDirect}
	if winner.KillSwitch {
		proxy.FallbackAction = domain.RuleActionBlock
	}
	for _, p := range profiles {
		if p.ID != winner.ProxyID {
			continue
		}
		proxy.Name = p.Name
		switch {
		case !p.Enabled:
			proxy.Availability = RouteProxyDisabled
		case !out.Engine.Running || !out.Engine.CaptureAlive:
			proxy.Availability = RouteProxyNotChecked
		default:
			proxy.Availability = RouteProxyUnavailable
			for _, status := range s.ProfileStatuses() {
				if status.ProfileID == p.ID && status.State == domain.ProxyConnected {
					proxy.Availability = RouteProxyConnected
					break
				}
			}
		}
		break
	}
	out.Proxy = proxy
	return out, nil
}
