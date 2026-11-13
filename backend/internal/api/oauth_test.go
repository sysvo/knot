package api

import "testing"

// TestPickVerifiedEmail covers the rule that decides whether an OAuth identity
// may be linked to an existing account. Getting this wrong is an account-takeover
// path, so the negative cases matter more than the positive ones.
func TestPickVerifiedEmail(t *testing.T) {
	cases := []struct {
		name     string
		list     []providerEmail
		wantMail string
		wantOK   bool
	}{
		{
			name: "primary verified wins",
			list: []providerEmail{
				{Email: "second@example.com", Verified: true},
				{Email: "primary@example.com", Primary: true, Verified: true},
			},
			wantMail: "primary@example.com",
			wantOK:   true,
		},
		{
			name: "unverified primary is not used",
			list: []providerEmail{
				{Email: "primary@example.com", Primary: true},
				{Email: "other@example.com", Verified: true},
			},
			wantMail: "other@example.com",
			wantOK:   true,
		},
		{
			name: "nothing verified yields nothing",
			list: []providerEmail{
				{Email: "primary@example.com", Primary: true},
				{Email: "other@example.com"},
			},
			wantMail: "",
			wantOK:   false,
		},
		{
			name:     "empty list yields nothing",
			list:     nil,
			wantMail: "",
			wantOK:   false,
		},
		{
			name:     "blank address is ignored even when flagged verified",
			list:     []providerEmail{{Email: "", Primary: true, Verified: true}},
			wantMail: "",
			wantOK:   false,
		},
	}

	for _, tc := range cases {
		mail, ok := pickVerifiedEmail(tc.list)
		if mail != tc.wantMail || ok != tc.wantOK {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, mail, ok, tc.wantMail, tc.wantOK)
		}
	}
}
