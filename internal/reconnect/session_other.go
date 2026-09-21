//go:build !windows

package reconnect

// NewSessionDetector returns a no-op detector on platforms without a session
// model (or where the sleep/wake detector already covers resume). The resume
// channel is nil, so the reconnect monitor's select simply never fires on it
// and behaviour is unchanged from before this feature existed.
func NewSessionDetector() SessionDetector {
	return &noOpSessionDetector{}
}

type noOpSessionDetector struct{}

func (n *noOpSessionDetector) Start()                      {}
func (n *noOpSessionDetector) Stop()                       {}
func (n *noOpSessionDetector) ResumeChan() <-chan struct{} { return nil }
