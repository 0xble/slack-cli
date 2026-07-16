package slack

import "testing"

func TestResolveUserGroupMentionsFromMap(t *testing.T) {
	got, count := ResolveUserGroupMentionsFromMap("Hey @Team, email a@team.com", map[string]string{"team": "S123"})
	if got != "Hey <!subteam^S123>, email a@team.com" || count != 1 {
		t.Fatalf("unexpected result %q count=%d", got, count)
	}
}
