package reach

import (
	"fmt"
	"strings"
)

// State is the overall verdict.
type State int

// The zero State is StateUnknown, so an empty Assessment claims nothing.
const (
	StateUnknown State = iota
	StateOK
	StateBlocked
	StateNotListening
)

// Cause says why the port is not reachable.
type Cause int

// Causes of a problem; CauseNone belongs to OK and CauseUnknown to unknown.
const (
	CauseNone Cause = iota
	CauseUnknown
	CausePublicNetwork
	CauseNoRule
	CauseBlockRule
	CauseOtherProgramOnPort
	CauseBindFailed
)

// Fix is the remedy the UI offers.
type Fix int

// Remedies. FixOpenNetworkSettings and FixAllowOnPublic come as a pair on a
// Public network: switching it to Private is the primary one.
const (
	FixNone Fix = iota
	FixAllowFirewall
	FixOpenNetworkSettings
	FixAllowOnPublic
)

// Assessment is the classifier's verdict with what the UI and the
// diagnostics need to explain it.
type Assessment struct {
	State State
	Cause Cause
	// Profile names the network profile the verdict is about.
	Profile string
	// Detail is a short reason: why the check could not tell, or the
	// listen failure.
	Detail string
	// Fix is the primary remedy and AltFix the second one, if any.
	Fix, AltFix Fix
	// Owner is the process that holds the port, when known.
	Owner string
	// ActiveProfiles are the connected networks' profiles; Rules are the
	// names of the rules that matched.
	ActiveProfiles []string
	Rules          []string
}

// Problem reports whether the verdict is one the user must act on.
func (a Assessment) Problem() bool { return a.State == StateBlocked || a.State == StateNotListening }

func unknown(detail string) Assessment {
	return Assessment{State: StateUnknown, Cause: CauseUnknown, Detail: detail}
}

// Assess classifies the inputs. It never says "blocked" on incomplete
// information: anything it cannot decide is StateUnknown.
func Assess(in Inputs) Assessment {
	if in.Listen.Attempted && !in.Listen.Listening {
		return notListening(in)
	}
	profiles, ok := activeProfiles(in.Profiles)
	if !ok || len(in.Firewall) == 0 {
		return unknown("Windows did not list the connected networks")
	}
	return aggregate(in, profiles)
}

// notListening is the verdict when the server has no listening socket.
func notListening(in Inputs) Assessment {
	a := Assessment{State: StateNotListening, Cause: CauseBindFailed, Detail: in.Listen.Err}
	for _, o := range in.Owners {
		if o.ProcessID != in.OwnPID && o.ProcessName != "" {
			a.Cause, a.Owner = CauseOtherProgramOnPort, o.ProcessName
			break
		}
	}
	return a
}

// activeProfile is a connected network's firewall profile.
type activeProfile struct {
	name string
	bit  int
}

// activeProfiles maps the connected networks to their firewall profiles,
// each once. ok is false when none is connected or one has a category the
// classifier does not know.
func activeProfiles(conns []ConnProfile) (out []activeProfile, ok bool) {
	seen := map[string]bool{}
	for _, c := range conns {
		if strings.EqualFold(c.IPv4Connectivity, "Disconnected") {
			continue
		}
		p, known := profileOfCategory(c.NetworkCategory)
		if !known {
			return nil, false
		}
		if !seen[p.name] {
			seen[p.name] = true
			out = append(out, p)
		}
	}
	return out, len(out) > 0
}

func profileOfCategory(category string) (activeProfile, bool) {
	switch strings.ToLower(category) {
	case "public":
		return activeProfile{ProfilePublic, profilePublic}, true
	case "private":
		return activeProfile{ProfilePrivate, profilePrivate}, true
	case "domainauthenticated", "domain":
		return activeProfile{ProfileDomain, profileDomain}, true
	}
	return activeProfile{}, false
}

// profileResult is the verdict for one active profile.
type profileResult struct {
	profile   activeProfile
	reachable tri
	matched   []string
	blockers  []Rule
}

// aggregate combines the per-profile results: every active profile must be
// covered, and the verdict names the worst one that is not.
func aggregate(in Inputs, profiles []activeProfile) Assessment {
	a := Assessment{State: StateOK, Cause: CauseNone}
	var blocked []profileResult
	undecided := false
	for _, p := range profiles {
		r := evalProfile(in, p)
		a.ActiveProfiles = append(a.ActiveProfiles, p.name)
		a.Rules = appendUnique(a.Rules, r.matched...)
		switch r.reachable {
		case no:
			blocked = append(blocked, r)
		case unsure:
			undecided = true
		}
	}
	if len(blocked) > 0 {
		return blockedAssessment(a, blocked)
	}
	if undecided {
		a.State, a.Cause, a.Detail = StateUnknown, CauseUnknown, "Some firewall rules could not be evaluated"
	}
	return a
}

// blockedAssessment names the cause: a blocking rule first, then a Public
// network without a rule, then a missing rule.
func blockedAssessment(a Assessment, blocked []profileResult) Assessment {
	a.State = StateBlocked
	worst := blocked[0]
	for _, r := range blocked {
		if len(r.blockers) > 0 {
			worst = r
			break
		}
		if r.profile.bit == profilePublic {
			worst = r
		}
	}
	a.Profile = worst.profile.name
	switch {
	case len(worst.blockers) > 0:
		a.Cause, a.Fix = CauseBlockRule, FixAllowFirewall
		for _, b := range worst.blockers {
			a.Detail = joinNames(a.Detail, ruleLabel(b))
			if !isProgramRule(b) {
				a.Fix = FixNone // a rule for every program: only the user can lift it
			}
		}
	case worst.profile.bit == profilePublic:
		a.Cause, a.Fix, a.AltFix = CausePublicNetwork, FixOpenNetworkSettings, FixAllowOnPublic
	default:
		a.Cause, a.Fix = CauseNoRule, FixAllowFirewall
	}
	return a
}

// evalProfile decides whether port is reachable on one profile.
func evalProfile(in Inputs, p activeProfile) profileResult {
	res := profileResult{profile: p, reachable: unsure}
	fw, found := firewallFor(in.Firewall, p.name)
	if !found {
		return res
	}
	if strings.EqualFold(fw.Enabled, "False") || strings.EqualFold(fw.DefaultInboundAction, "Allow") {
		res.reachable = yes
		return res
	}
	allow, block := matchingRules(in, p, &res)
	switch {
	case len(res.blockers) > 0:
		res.reachable = no
	case block == unsure:
		res.reachable = unsure
	case allow == yes:
		res.reachable = yes
	case allow == unsure:
		res.reachable = unsure
	default:
		res.reachable = no
	}
	return res
}

func firewallFor(all []FirewallProfile, name string) (FirewallProfile, bool) {
	for _, f := range all {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return FirewallProfile{}, false
}

// matchingRules folds the rules that concern the port on profile p into
// whether an allow rule applies and whether a block rule might; definite
// block rules are collected in res.
func matchingRules(in Inputs, p activeProfile, res *profileResult) (allow, block tri) {
	for _, r := range in.Rules {
		if !matchesTCP(r.Protocol) || !matchesPort(r.LocalPort, in.Port) || !profileCovers(r.Profile, p.bit) {
			continue
		}
		applies := both(matchesProgram(r.Program, in.ExePath), coversSubnet(r.RemoteAddress, in.LocalNet))
		if applies == no {
			continue
		}
		if applies == yes {
			res.matched = appendUnique(res.matched, ruleLabel(r))
		}
		switch {
		case r.Action == actionBlock && applies == yes:
			res.blockers = append(res.blockers, r)
		case r.Action == actionBlock:
			block = unsure
		case r.Action == actionAllow:
			allow = maxTri(allow, applies)
		}
	}
	return allow, block
}

// both is the answer to "all of these hold": no wins, then unsure.
func both(a, b tri) tri {
	if a == no || b == no {
		return no
	}
	if a == unsure || b == unsure {
		return unsure
	}
	return yes
}

// maxTri prefers yes over unsure over no.
func maxTri(a, b tri) tri {
	if a == yes || b == yes {
		return yes
	}
	if a == unsure || b == unsure {
		return unsure
	}
	return no
}

func ruleLabel(r Rule) string {
	if r.DisplayName != "" {
		return r.DisplayName
	}
	return r.Name
}

// isProgramRule reports whether a rule is tied to one program (the kind
// Windows creates when its prompt is dismissed), which the fix removes.
func isProgramRule(r Rule) bool {
	p := strings.TrimSpace(r.Program)
	return p != "" && !strings.EqualFold(p, "any")
}

func appendUnique(list []string, items ...string) []string {
	for _, item := range items {
		found := false
		for _, have := range list {
			found = found || have == item
		}
		if !found {
			list = append(list, item)
		}
	}
	return list
}

func joinNames(list, name string) string {
	if list == "" {
		return name
	}
	return list + ", " + name
}

// String is the verdict in one line for logs.
func (a Assessment) String() string {
	return fmt.Sprintf("state=%s cause=%s profile=%q detail=%q", a.State, a.Cause, a.Profile, a.Detail)
}
