package app

import (
	"fmt"
	"sort"
	"strings"
)

// Field editing support for the tunnel editor's "fields" view. The frontend
// sends per-section key/value maps using the NATIVE .conf key spelling
// (PrivateKey, AllowedIPs, Jc, …); we rewrite the conf TEXT in place — same
// philosophy as SetHookInText: a targeted rewrite that never re-serializes
// the document, so hand-ordered lines, comments, and unknown keys survive.

// TunnelFields is the field-editor model for one tunnel config. Values are
// raw strings; list fields (Address/DNS/AllowedIPs) stay comma-joined. An
// empty value means "remove this key from the conf".
type TunnelFields struct {
	Interface map[string]string   `json:"interface"`
	Peers     []map[string]string `json:"peers"`
}

type confSection int

const (
	sectionNone confSection = iota
	sectionInterface
	sectionPeer
)

func classifyHeader(trimmed string) confSection {
	switch {
	case strings.EqualFold(trimmed, "[interface]"):
		return sectionInterface
	case strings.EqualFold(trimmed, "[peer]"):
		return sectionPeer
	default:
		return sectionNone
	}
}

// GetTunnelFields extracts the field-editor model from a .conf text using
// pure text scanning.
func (s *TunnelService) GetTunnelFields(content string) (*TunnelFields, error) {
	fields := &TunnelFields{Interface: map[string]string{}}
	section := sectionNone
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			section = classifyHeader(trimmed)
			if section == sectionPeer {
				fields.Peers = append(fields.Peers, map[string]string{})
			}
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		val := strings.TrimSpace(trimmed[eq+1:])
		switch section {
		case sectionInterface:
			fields.Interface[key] = val
		case sectionPeer:
			if len(fields.Peers) > 0 {
				fields.Peers[len(fields.Peers)-1][key] = val
			}
		}
	}
	return fields, nil
}

// SetTunnelFields applies the field-editor model onto a .conf text. Managed
// keys are replaced in place (or deleted when the model value is empty);
// keys absent from the text are appended at the end of their section.
// Comments, blank lines, unknown keys, and ordering are preserved. The
// model's Peers array maps positionally onto the [Peer] sections; extra
// conf peers are untouched, extra model peers are ignored.
func (s *TunnelService) SetTunnelFields(content string, fields *TunnelFields) (string, error) {
	if fields == nil {
		return content, nil
	}
	models := append([]map[string]string{fields.Interface}, fields.Peers...)
	for _, m := range models {
		for key := range m {
			if key == "" || strings.ContainsAny(key, "[]=\n\r") {
				return "", fmt.Errorf("invalid key %q", key)
			}
		}
	}

	type span struct {
		start, end int
		target     map[string]string
	}
	var spans []span
	var res []string
	peerIdx := -1
	var target map[string]string
	secStart := -1

	flushSpan := func(endIdx int) {
		if secStart >= 0 {
			spans = append(spans, span{secStart, endIdx, target})
			secStart = -1
			target = nil
		}
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			flushSpan(len(res))
			res = append(res, line)
			switch classifyHeader(trimmed) {
			case sectionInterface:
				target = fields.Interface
				secStart = len(res) - 1
			case sectionPeer:
				peerIdx++
				if peerIdx < len(fields.Peers) {
					target = fields.Peers[peerIdx]
				}
				secStart = len(res) - 1
			default:
				target = nil
			}
			continue
		}
		eq := strings.Index(trimmed, "=")
		if target != nil && eq >= 0 {
			key := strings.TrimSpace(trimmed[:eq])
			if val, managed := lookupManagedKey(target, key); managed {
				if val != "" {
					res = append(res, key+" = "+val)
				} // empty model value → delete the line
				continue
			}
		}
		res = append(res, line)
	}
	flushSpan(len(res))

	// Insert missing keys, last span first so earlier indices stay valid.
	for i := len(spans) - 1; i >= 0; i-- {
		sp := spans[i]
		if sp.target == nil {
			continue
		}
		present := map[string]bool{}
		for j := sp.start; j < sp.end; j++ {
			line := strings.TrimSpace(res[j])
			eq := strings.Index(line, "=")
			if eq < 0 {
				continue
			}
			present[strings.ToLower(strings.TrimSpace(line[:eq]))] = true
		}
		var missing []string
		for k, v := range sp.target {
			if v == "" || present[strings.ToLower(k)] {
				continue
			}
			missing = append(missing, k)
		}
		sort.Strings(missing)
		for _, k := range missing {
			res = append(res, "")
			copy(res[sp.end+1:], res[sp.end:])
			res[sp.end] = k + " = " + sp.target[k]
			sp.end++
		}
	}
	return strings.Join(res, "\n"), nil
}

// lookupManagedKey finds a model value for a conf key, case-insensitively.
func lookupManagedKey(target map[string]string, key string) (string, bool) {
	for k, v := range target {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}
