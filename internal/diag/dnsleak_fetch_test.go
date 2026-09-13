package diag

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParsePublicResolversSmoke feeds a realistic slice of the
// public-dns.info nameservers.json feed (mix of healthy, errored and
// invalid-IP entries) and asserts the parser returns a capped, de-duplicated,
// reliability-sorted list.
func TestParsePublicResolversSmoke(t *testing.T) {
	feed := `[
		{"ip":"1.1.1.1","reliability":9.8,"error":""},
		{"ip":"8.8.8.8","reliability":9.5,"error":""},
		{"ip":"9.9.9.9","reliability":0.1,"error":""},
		{"ip":"1.1.1.1","reliability":9.9,"error":""},
		{"ip":"","reliability":9.9,"error":""},
		{"ip":"not-an-ip","reliability":9.9,"error":""},
		{"ip":"208.67.222.222","reliability":0.0,"error":"timeout"}
	]`
	out, err := parsePublicResolvers(json.NewDecoder(strings.NewReader(feed)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 usable resolvers (dedup of 1.1.1.1), got %d: %v", len(out), out)
	}
	// Highest reliability first: 1.1.1.1 (9.8/9.9 folded to one entry, 8.8.8.8 (9.5), 9.9.9.9 (0.1).
	if out[0] != "1.1.1.1" || out[1] != "8.8.8.8" || out[2] != "9.9.9.9" {
		t.Fatalf("unexpected order/contents: %v", out)
	}
}

// TestParsePublicResolversStopsEarly proves the parser does NOT need the whole
// feed: a malformed entry after the first publicFetchLimit valid ones must not
// cause an error (it breaks out early instead of decoding the bad tail).
func TestParsePublicResolversStopsEarly(t *testing.T) {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < publicFetchLimit; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"ip":"203.0.113.` + itoa(i) + `","reliability":9.0,"error":""}`)
	}
	// A broken entry following the valid ones — if the parser tried to read to
	// the end it would return a "parse entry" error; with early-stop it must
	// return cleanly.
	b.WriteString(`,"this is not an object"`)
	b.WriteString("]")

	out, err := parsePublicResolvers(json.NewDecoder(strings.NewReader(b.String())))
	if err != nil {
		t.Fatalf("expected clean early stop, got error: %v", err)
	}
	if len(out) != publicFetchLimit {
		t.Fatalf("expected exactly publicFetchLimit (%d) resolvers, got %d", publicFetchLimit, len(out))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
