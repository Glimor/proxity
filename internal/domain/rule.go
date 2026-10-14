package domain

import (
	"path/filepath"
	"strings"
	"time"
)

type RuleKind string

const (
	RuleKindExe    RuleKind = "exe"
	RuleKindFolder RuleKind = "folder"
)

type RuleAction string

const (
	RuleActionProxy  RuleAction = "proxy"
	RuleActionDirect RuleAction = "direct"
	RuleActionBlock  RuleAction = "block"
)

type Rule struct {
	ID             string
	Name           string
	Kind           RuleKind
	Path           string // canonicalized absolute path
	PathFold       string // casefolded copy used for matching
	Action         RuleAction
	ProxyID        string
	Enabled        bool
	KillSwitch     bool
	IncludeSubdirs bool
	Priority       int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CanonicalPath returns a cleaned absolute path with an upper-cased volume
// letter and backslash separators, suitable for storage and matching.
func CanonicalPath(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "/", `\`))
	if p == "" {
		return "", NewError("rule.empty_path", "Rule path is empty", "")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", WrapError("rule.bad_path", "Rule path is not a valid path", err)
	}
	clean := filepath.Clean(strings.ReplaceAll(abs, "/", `\`))
	vol := filepath.VolumeName(clean)
	return strings.ToUpper(vol) + clean[len(vol):], nil
}

func FoldPath(p string) string { return strings.ToUpper(p) }

// CanonicalizeRulePath fills Path and PathFold from the raw path input.
func CanonicalizeRulePath(r *Rule, rawPath string) error {
	c, err := CanonicalPath(rawPath)
	if err != nil {
		return err
	}
	r.Path = c
	r.PathFold = FoldPath(c)
	return nil
}

func (r *Rule) Matches(exePathFold string) bool {
	if !r.Enabled {
		return false
	}
	switch r.Kind {
	case RuleKindExe:
		return r.PathFold == exePathFold
	case RuleKindFolder:
		if r.PathFold == filepath.Dir(exePathFold) {
			return true
		}
		return r.IncludeSubdirs && strings.HasPrefix(exePathFold, r.PathFold+`\`)
	}
	return false
}

// MatchRule resolves the winning rule for a casefolded exe path: an exact exe
// rule always wins, otherwise the deepest matching folder, ties broken by the
// higher Priority. Returns nil when nothing matches.
func MatchRule(rules []Rule, exePathFold string) *Rule {
	var best *Rule
	bestLen := -1
	for i := range rules {
		r := &rules[i]
		if !r.Matches(exePathFold) {
			continue
		}
		if r.Kind == RuleKindExe {
			if best == nil || best.Kind != RuleKindExe || r.Priority > best.Priority {
				best = r
			}
			continue
		}
		if best != nil && best.Kind == RuleKindExe {
			continue
		}
		l := len(r.PathFold)
		if l > bestLen || (l == bestLen && best != nil && r.Priority > best.Priority) {
			best = r
			bestLen = l
		}
	}
	return best
}
