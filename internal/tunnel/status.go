package tunnel

import (
	"fmt"
	"time"

	"github.com/imonior/wireguide-plus/internal/domain"
	"golang.zx2c4.com/wireguard/wgctrl"
)

// Re-export the canonical connection status + state types from the domain
// package so existing callers (`tunnel.ConnectionStatus`, `tunnel.StateConnected`)
// keep compiling. There is a single underlying type — methods defined on the
// domain type work transparently through these aliases.
type (
	ConnectionStatus = domain.ConnectionStatus
	State            = domain.State
)

const (
	StateDisconnected = domain.StateDisconnected
	StateConnecting   = domain.StateConnecting
	StateConnected    = domain.StateConnected
	StateError        = domain.StateError
)

// handshakeStaleThreshold is how long a connected tunnel may go without a
// successful WireGuard handshake before we report it as stale. Peers with
// PersistentKeepalive re-handshake on that interval (commonly 25s), so a
// healthy tunnel refreshes well within this window; a tunnel whose upstream
// dropped (local NIC/interface stay up, peer unreachable) stops handshaking
// and the last handshake timestamp freezes — after this window it is stale.
// 150s (6x the typical 25s keepalive) catches a dead upstream without
// false-positiving on a healthy tunnel.
const handshakeStaleThreshold = 150 * time.Second

// markHandshakeStale sets status.HandshakeStale. A freshly-connected tunnel
// that simply hasn't handshaked yet is NOT flagged — only one that used to be
// able to reach its peer and now cannot (up past the threshold, no handshake
// within it), or one that has been connected past the threshold yet never
// handshaked at all (dead peer / NAT hole closed before the first handshake).
// The latter case is aged from ConnectedAt so the UI shows the degraded state
// in step with the reconnect monitor, which also recovers it.
// 标记 HandshakeStale。刚连接、尚未握手的隧道不标记；只有原本能抵达对端现在
// 不能（连接超阈值且窗口内无握手），或连接已超阈值却从未握手（对端死 / NAT 洞在
// 首次握手前关闭）才标记。后者以 ConnectedAt 为基线，使 UI 劣化态与负责恢复的
// 重连监视器保持同步。
func markHandshakeStale(s *domain.ConnectionStatus) {
	if s == nil {
		return
	}
	if s.LastHandshakeTime.IsZero() {
		if s.ConnectedAt.IsZero() {
			return
		}
		s.HandshakeStale = time.Since(s.ConnectedAt) > handshakeStaleThreshold
		return
	}
	if time.Since(s.ConnectedAt) < handshakeStaleThreshold {
		return
	}
	s.HandshakeStale = time.Since(s.LastHandshakeTime) > handshakeStaleThreshold
}

// GetStatus queries the current status of a WireGuard interface.
//
// NOTE: This creates a new wgctrl client on every call. If performance becomes
// an issue (e.g. sub-second polling), consider caching the client at the Manager
// level. For now, a fresh client per call is fine — wgctrl.New() is cheap
// (opens a netlink/UAPI socket) and avoids stale-connection edge cases.
func GetStatus(ifaceName string, tunnelName string, connectedAt time.Time) (*ConnectionStatus, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("creating wgctrl client: %w", err)
	}
	defer client.Close()

	dev, err := client.Device(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("querying device %s: %w", ifaceName, err)
	}

	status := &ConnectionStatus{
		State:         StateConnected,
		TunnelName:    tunnelName,
		InterfaceName: ifaceName,
		ConnectedAt:   connectedAt,
		Duration:      domain.FormatDuration(time.Since(connectedAt)),
	}

	// Aggregate stats from all peers
	for _, peer := range dev.Peers {
		status.RxBytes += peer.ReceiveBytes
		status.TxBytes += peer.TransmitBytes

		if !peer.LastHandshakeTime.IsZero() {
			if status.LastHandshakeTime.IsZero() || peer.LastHandshakeTime.After(status.LastHandshakeTime) {
				status.LastHandshakeTime = peer.LastHandshakeTime
			}
		}

		if peer.Endpoint != nil && status.Endpoint == "" {
			status.Endpoint = peer.Endpoint.String()
		}
	}

	if !status.LastHandshakeTime.IsZero() {
		status.LastHandshake = domain.FormatDuration(time.Since(status.LastHandshakeTime))
		// Absolute timestamp so the GUI can age the handshake itself between
		// broadcasts, rather than trusting the string above which goes stale
		// the moment the status stream does.
		status.LastHandshakeUnix = status.LastHandshakeTime.Unix()
	}
	// Flag a tunnel whose peer stopped answering while the link stayed up
	// (dead upstream). The GUI renders this as a degraded state.
	markHandshakeStale(status)

	return status, nil
}
