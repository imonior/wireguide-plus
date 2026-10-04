package ipc

import (
	"testing"
)

// TestOwnerGrantScopesToTheOwnerSID: the GUI's read+write grant names exactly
// the spawning user, nothing else.
func TestOwnerGrantScopesToTheOwnerSID(t *testing.T) {
	for _, sid := range []string{
		"S-1-5-21-3623811015-3361044348-30300820-1013", // domain / Microsoft-account user
		"S-1-5-21-1-2-3-500",                           // built-in Administrator is still a user SID
	} {
		got, err := ownerGrant(sid)
		if err != nil {
			t.Errorf("real user SID %q rejected: %v", sid, err)
			continue
		}
		if want := "(A;;GRGW;;;" + sid + ")"; got != want {
			t.Errorf("grant = %q, want %q", got, want)
		}
	}
}

// TestOwnerGrantRefusesUnscoped: no owner, an SDDL alias, a shared principal,
// or anything that isn't one plain numeric SID must be an error. HEAD answered
// the empty case with the Interactive Users grant, which handed a SYSTEM
// helper to every logged-on account on the machine (issue #20), and the rest
// of these are the ways a caller could widen that grant again.
func TestOwnerGrantRefusesUnscoped(t *testing.T) {
	for _, sid := range []string{
		"",             // no owner told to the helper
		"IU",           // Interactive Users as an SDDL alias
		"BA",           // Built-in Administrators
		"SY",           // SYSTEM
		"s-1-5-21-1-2", // lowercase is not canonical
		"S-1-5",        // authority with no relative identifier
		"S-1-5-4",      // Interactive Users, numeric form
		"S-1-1-0",      // Everyone
		"S-1-5-7",      // Authenticated Users
		"S-1-5-32-544", // Built-in Administrators
		"S-1-5-21-abc", // non-numeric component
		"S-1-5-21-1013;X",
		"S-1-5-21-1013)(A;;GRGW;;;IU", // extra access block through the SID field
		"S-1-5-21-1013 ",
		" S-1-5-21-1013",
	} {
		grant, err := ownerGrant(sid)
		if err == nil {
			t.Errorf("owner SID %q accepted, grant %q", sid, grant)
			continue
		}
		if grant != "" {
			t.Errorf("owner SID %q rejected but still returned a grant: %q", sid, grant)
		}
	}
}
