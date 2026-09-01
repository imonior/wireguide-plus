//go:build darwin

package update

import "testing"

func TestApplescriptQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/tmp/plain.sh", `"/tmp/plain.sh"`},
		{`/tmp/quote".sh`, `"/tmp/quote\".sh"`},
		{`/tmp/back\slash.sh`, `"/tmp/back\\slash.sh"`},
	}
	for _, c := range cases {
		if got := applescriptQuote(c.in); got != c.want {
			t.Errorf("applescriptQuote(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}
