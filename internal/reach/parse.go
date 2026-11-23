package reach

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// rawFacts is the JSON the collection script prints. Every list may arrive
// as an array, as one bare object (ConvertTo-Json does not wrap a single
// item) or as null.
type rawFacts struct {
	Profiles oneOrMany[ConnProfile]
	Firewall oneOrMany[FirewallProfile]
	Rules    oneOrMany[rawRule]
	Owner    oneOrMany[rawOwner]
}

type rawRule struct {
	Name, DisplayName, Action, Protocol, Program string
	Profile                                      flexibleInt
	LocalPort                                    oneOrMany[string]
	RemoteAddress                                oneOrMany[string]
}

type rawOwner struct {
	ProcessName string
	ProcessId   int
}

// oneOrMany decodes a JSON array, a single value or null into a slice.
type oneOrMany[T any] []T

func (m *oneOrMany[T]) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	switch {
	case len(data) == 0 || string(data) == "null":
		*m = nil
	case data[0] == '[':
		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		*m = items
	default:
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		*m = []T{item}
	}
	return nil
}

// flexibleInt decodes a number or a numeric string.
type flexibleInt int

func (f *flexibleInt) UnmarshalJSON(data []byte) error {
	text := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if text == "" || text == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("not a number: %q", text)
	}
	*f = flexibleInt(n)
	return nil
}

// ParseFacts decodes the collection script's output. Empty output and
// malformed JSON are errors, so the caller reports "couldn't check" rather
// than guessing from nothing.
func ParseFacts(output []byte) (Facts, error) {
	output = bytes.TrimPrefix(bytes.TrimSpace(output), []byte("\xef\xbb\xbf"))
	if len(output) == 0 || string(output) == "null" {
		return Facts{}, fmt.Errorf("the network query printed nothing")
	}
	var raw rawFacts
	if err := json.Unmarshal(output, &raw); err != nil {
		return Facts{}, fmt.Errorf("the network query printed something unexpected: %w", err)
	}
	facts := Facts{Profiles: raw.Profiles, Firewall: raw.Firewall}
	for _, r := range raw.Rules {
		facts.Rules = append(facts.Rules, r.rule())
	}
	for _, o := range raw.Owner {
		facts.Owners = append(facts.Owners, PortOwner{ProcessName: o.ProcessName, ProcessID: o.ProcessId})
	}
	return facts, nil
}

func (r rawRule) rule() Rule {
	return Rule{
		Name: r.Name, DisplayName: r.DisplayName, Action: normalizeAction(r.Action),
		Profile: int(r.Profile), Protocol: r.Protocol, LocalPort: r.LocalPort,
		RemoteAddress: r.RemoteAddress, Program: r.Program,
	}
}

// Rule actions as the classifier names them.
const (
	actionAllow = "Allow"
	actionBlock = "Block"
)

// normalizeAction maps the numeric spellings (2 allow, 4 block) of an
// action to its name.
func normalizeAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "allow", "2":
		return actionAllow
	case "block", "4":
		return actionBlock
	}
	return action
}
