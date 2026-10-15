package endpoint

import "testing"

// m is a short constructor for a Measurement used throughout the tests.
// m 是测试里反复用到的 Measurement 简写构造器。
func m(addr string, reachable bool, latency float64) Measurement {
	return Measurement{Address: addr, Reachable: reachable, LatencyMs: latency}
}

// TestBetter_Failover: the current address is down while another answers.
// The switch is a failover and must happen immediately.
// TestBetter_Failover：当前地址已死、另有候选应答，应立刻故障切换。
func TestBetter_Failover(t *testing.T) {
	ms := []Measurement{
		m("a:51820", false, 0),
		m("b:51820", true, 40),
	}
	chosen, failover := Better(ms, "a:51820", DefaultOpts())
	if chosen != "b:51820" || !failover {
		t.Fatalf("want b:51820 failover, got %q failover=%v", chosen, failover)
	}
}

// TestBetter_Improvement: current is slower than the threshold and the
// alternative is faster by more than the margin, so it is an improvement.
// TestBetter_Improvement：当前慢于阈值、备选快出超过幅度，属于择优。
func TestBetter_Improvement(t *testing.T) {
	ms := []Measurement{
		m("a:51820", true, 200),
		m("b:51820", true, 100),
	}
	chosen, failover := Better(ms, "a:51820", DefaultOpts())
	if chosen != "b:51820" || failover {
		t.Fatalf("want b:51820 improvement, got %q failover=%v", chosen, failover)
	}
}

// TestBetter_CurrentGoodEnough: current is under the threshold, so even a
// much faster alternative must NOT trigger a switch — no churn.
// TestBetter_CurrentGoodEnough：当前低于阈值，即便备选快很多也不切——不折腾。
func TestBetter_CurrentGoodEnough(t *testing.T) {
	ms := []Measurement{
		m("a:51820", true, 100),
		m("b:51820", true, 10),
	}
	chosen, failover := Better(ms, "a:51820", DefaultOpts())
	if chosen != "" || failover {
		t.Fatalf("want no switch, got %q failover=%v", chosen, failover)
	}
}

// TestBetter_NotBetterByMargin: current is over the threshold but the
// alternative is not faster by the required margin, so it is kept.
// TestBetter_NotBetterByMargin：当前超阈值但备选优势未达幅度，保持。
func TestBetter_NotBetterByMargin(t *testing.T) {
	ms := []Measurement{
		m("a:51820", true, 200),
		m("b:51820", true, 170),
	}
	chosen, failover := Better(ms, "a:51820", DefaultOpts())
	if chosen != "" || failover {
		t.Fatalf("want no switch, got %q failover=%v", chosen, failover)
	}
}

// TestBetter_NoFallback: current is reachable but every other candidate is
// down, so there is nowhere better to go.
// TestBetter_NoFallback：当前可达、其它候选全死，无处可去。
func TestBetter_NoFallback(t *testing.T) {
	ms := []Measurement{
		m("a:51820", true, 300),
		m("b:51820", false, 0),
	}
	chosen, failover := Better(ms, "a:51820", DefaultOpts())
	if chosen != "" || failover {
		t.Fatalf("want no switch, got %q failover=%v", chosen, failover)
	}
}

// TestBetter_DefaultsApplied: a zero Opts must fall back to the package
// defaults (threshold 150, margin 30) rather than switching blindly.
// TestBetter_DefaultsApplied：零值 Opts 必须回退到包默认值，而非盲切。
func TestBetter_DefaultsApplied(t *testing.T) {
	ms := []Measurement{
		m("a:51820", true, 200),
		m("b:51820", true, 100),
	}
	chosen, failover := Better(ms, "a:51820", Opts{})
	if chosen != "b:51820" || failover {
		t.Fatalf("want b:51820 improvement with defaults, got %q failover=%v", chosen, failover)
	}
}

// TestBest: lowest-latency reachable wins; ties break on input (preference)
// order; nothing reachable yields "".
// TestBest：延迟最低的可达候选胜出；平局按输入顺序（偏好）裁决；全不可达返回 ""。
func TestBest(t *testing.T) {
	ms := []Measurement{
		m("a:51820", true, 50),
		m("b:51820", true, 20),
		m("c:51820", false, 0),
	}
	if got := Best(ms); got != "b:51820" {
		t.Fatalf("want b:51820, got %q", got)
	}
	ms2 := []Measurement{
		m("a:51820", true, 30),
		m("b:51820", true, 30),
	}
	if got := Best(ms2); got != "a:51820" {
		t.Fatalf("want a:51820 (tie → order), got %q", got)
	}
	if got := Best([]Measurement{m("a:51820", false, 0)}); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

// TestNormalize: trims, drops empties, de-duplicates, preserves order.
// TestNormalize：去空白、丢空项、去重，并保持顺序。
func TestNormalize(t *testing.T) {
	in := []string{" b:51820 ", "", "a:51820", "a:51820", " b:51820 "}
	out := Normalize(in)
	want := []string{"b:51820", "a:51820"}
	if len(out) != len(want) {
		t.Fatalf("want %v got %v", want, out)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("want %v got %v", want, out)
		}
	}
}

// TestHost: splits host:port; passes a bare host through unchanged.
// TestHost：拆分 host:port；无端口的裸 host 原样返回。
func TestHost(t *testing.T) {
	if got := Host("example.com:51820"); got != "example.com" {
		t.Fatalf("want example.com, got %q", got)
	}
	if got := Host("example.com"); got != "example.com" {
		t.Fatalf("want example.com (no port passthrough), got %q", got)
	}
}

// TestDefaultOpts: returns the package defaults.
// TestDefaultOpts：返回包级默认值。
func TestDefaultOpts(t *testing.T) {
	o := DefaultOpts()
	if o.ThresholdMs != DefaultThresholdMs || o.MarginPct != DefaultMarginPct {
		t.Fatalf("want defaults %d/%d, got %d/%d",
			DefaultThresholdMs, DefaultMarginPct, o.ThresholdMs, o.MarginPct)
	}
}
