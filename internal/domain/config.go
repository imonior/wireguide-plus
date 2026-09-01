// Package domain holds the core types of the WireGuide application.
// These are pure value objects with no external dependencies — they can be
// used freely from any package without creating import cycles.
package domain

import "net"

// Tunnel protocol identifiers. WireGuard is the default when Protocol is
// empty; AmneziaWG (AWG) is a fork of WireGuard with obfuscation parameters
// (Jc/Jmin/Jmax/S1-S4/H1-H4) and needs a different protocol backend.
const (
	ProtocolWireGuard = "wireguard"
	ProtocolAmneziaWG = "amneziawg"
)

// WireGuardConfig represents a complete WireGuard configuration file.
type WireGuardConfig struct {
	Name      string          `json:"name"`      // Tunnel name (derived from filename)
	Interface InterfaceConfig `json:"interface"` // [Interface] section
	Peers     []PeerConfig    `json:"peers"`     // [Peer] sections (1 or more)

	// Protocol selects the protocol backend: "" or "wireguard" = standard
	// WireGuard, "amneziawg" = AmneziaWG. It is derived from the config
	// content at parse time (presence of any AWG obfuscation key marks the
	// config as AWG) and is NOT part of the .conf serialization — the same
	// content always re-derives the same protocol.
	Protocol string `json:"protocol,omitempty"`

	// EnableScripts is injected at runtime by the GUI from user settings
	// (Settings → advanced → enable WireGuard scripts). It is NOT part of
	// the on-disk .conf serialization, but it MUST be transmitted over
	// IPC (hence json tag, not `json:"-"`): the helper's reconnect path
	// reuses the cached config and applies the same policy as the
	// initial connect.
	EnableScripts bool `json:"enable_scripts,omitempty"`
}

// InterfaceConfig represents the [Interface] section of a .conf file.
type InterfaceConfig struct {
	PrivateKey string   `json:"private_key"`           // Required: Base64-encoded 32-byte key
	Address    []string `json:"address"`               // Required: CIDR addresses (e.g., "10.0.0.2/24")
	DNS        []string `json:"dns,omitempty"`         // Optional: DNS servers and/or search domains
	MTU        int      `json:"mtu,omitempty"`         // Optional: 0 = auto-detect
	ListenPort int      `json:"listen_port,omitempty"` // Optional: 0 = random
	Table      string   `json:"table,omitempty"`       // Optional: routing table
	FwMark     string   `json:"fw_mark,omitempty"`     // Optional: firewall mark
	PreUp      string   `json:"pre_up,omitempty"`      // Optional: script before interface up
	PostUp     string   `json:"post_up,omitempty"`     // Optional: script after interface up
	PreDown    string   `json:"pre_down,omitempty"`    // Optional: script before interface down
	PostDown   string   `json:"post_down,omitempty"`   // Optional: script after interface down
	ExtraKeys  map[string]string `json:"extra_keys,omitempty"` // Unrecognized keys preserved for round-tripping

	// AmneziaWG obfuscation parameters (device-level). These are sent to
	// amneziawg-go via UAPI when Protocol == "amneziawg"; they are ignored
	// (and rejected by the validator) for standard WireGuard configs.
	// H1-H4 accept a single value or a "min-max" range string, matching
	// amneziawg-go's UintRange format.
	Jc   int    `json:"jc,omitempty"`    // Junk packet count (device-level)
	Jmin int    `json:"jmin,omitempty"`  // Junk packet min length
	Jmax int    `json:"jmax,omitempty"`  // Junk packet max length
	S1   int    `json:"s1,omitempty"`    // Init packet padding
	S2   int    `json:"s2,omitempty"`    // Response packet padding
	S3   int    `json:"s3,omitempty"`    // Cookie packet padding
	S4   int    `json:"s4,omitempty"`    // Transport packet padding
	H1   string `json:"h1,omitempty"`    // Init header range
	H2   string `json:"h2,omitempty"`    // Response header range
	H3   string `json:"h3,omitempty"`    // Cookie header range
	H4   string `json:"h4,omitempty"`    // Transport header range
}

// IsAmneziaWG reports whether this config uses the AmneziaWG protocol.
func (c *InterfaceConfig) IsAmneziaWG() bool {
	return c.Jc != 0 || c.Jmin != 0 || c.Jmax != 0 ||
		c.S1 != 0 || c.S2 != 0 || c.S3 != 0 || c.S4 != 0 ||
		c.H1 != "" || c.H2 != "" || c.H3 != "" || c.H4 != ""
}

// PeerConfig represents a [Peer] section of a .conf file.
type PeerConfig struct {
	PublicKey           string   `json:"public_key"`                     // Required: Base64-encoded 32-byte key
	PresharedKey        string   `json:"preshared_key,omitempty"`        // Optional
	Endpoint            string   `json:"endpoint,omitempty"`             // Optional: host:port
	AllowedIPs          []string `json:"allowed_ips"`                    // Required: CIDR list
	PersistentKeepalive int      `json:"persistent_keepalive,omitempty"` // Optional: seconds (0 = disabled)
	ExtraKeys           map[string]string `json:"extra_keys,omitempty"` // Unrecognized keys preserved for round-tripping
}

// HasScripts returns true if any Pre/PostUp/Down scripts are defined.
func (c *WireGuardConfig) HasScripts() bool {
	return c.Interface.PreUp != "" ||
		c.Interface.PostUp != "" ||
		c.Interface.PreDown != "" ||
		c.Interface.PostDown != ""
}

// Script represents a Pre/PostUp/Down hook command.
type Script struct {
	Hook    string `json:"hook"`    // "PreUp" | "PostUp" | "PreDown" | "PostDown"
	Command string `json:"command"` // Shell command to execute
}

// Scripts returns all defined script commands with their hook names.
func (c *WireGuardConfig) Scripts() []Script {
	var scripts []Script
	if c.Interface.PreUp != "" {
		scripts = append(scripts, Script{Hook: "PreUp", Command: c.Interface.PreUp})
	}
	if c.Interface.PostUp != "" {
		scripts = append(scripts, Script{Hook: "PostUp", Command: c.Interface.PostUp})
	}
	if c.Interface.PreDown != "" {
		scripts = append(scripts, Script{Hook: "PreDown", Command: c.Interface.PreDown})
	}
	if c.Interface.PostDown != "" {
		scripts = append(scripts, Script{Hook: "PostDown", Command: c.Interface.PostDown})
	}
	return scripts
}

// IsFullTunnel returns true if any peer routes all traffic (0.0.0.0/0 or ::/0).
func (c *WireGuardConfig) IsFullTunnel() bool {
	for _, peer := range c.Peers {
		for _, ip := range peer.AllowedIPs {
			_, cidr, err := net.ParseCIDR(ip)
			if err != nil {
				continue
			}
			ones, bits := cidr.Mask.Size()
			if ones == 0 && (bits == 32 || bits == 128) {
				return true
			}
		}
	}
	return false
}

// Endpoints returns all non-empty peer endpoints. Used for bypass route setup
// on full-tunnel mode — we need to add host routes for every peer endpoint,
// not just the first one (multi-peer site-to-site configs).
func (c *WireGuardConfig) Endpoints() []string {
	var eps []string
	for _, p := range c.Peers {
		if p.Endpoint != "" {
			eps = append(eps, p.Endpoint)
		}
	}
	return eps
}
