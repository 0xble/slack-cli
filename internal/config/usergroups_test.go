package config

import "testing"

func TestSetWorkspaceAuthPreservesUserGroupMappings(t *testing.T) {
	cfg := &Config{Workspaces: map[string]WorkspaceAuth{
		"example.slack.com": {
			Token:      "old-token",
			UserGroups: map[string]string{"team": "S123"},
		},
	}}
	cfg.SetWorkspaceAuth("example.slack.com", WorkspaceAuth{Token: "new-token", TeamID: "T123"})
	got := cfg.Workspaces["example.slack.com"]
	if got.Token != "new-token" || got.UserGroups["team"] != "S123" {
		t.Fatalf("expected refreshed auth and preserved user groups, got %+v", got)
	}
}

func TestUserGroupMappingsForWorkspace(t *testing.T) {
	cfg := &Config{
		CurrentWorkspace: "example.slack.com",
		Workspaces: map[string]WorkspaceAuth{
			"example.slack.com": {
				UserGroups: map[string]string{" @Team ": "S123"},
			},
		},
	}
	got := cfg.UserGroupMappings("")
	if got["team"] != "S123" {
		t.Fatalf("expected normalized team mapping, got %+v", got)
	}
	got["team"] = "changed"
	if cfg.Workspaces["example.slack.com"].UserGroups[" @Team "] != "S123" {
		t.Fatal("expected defensive copy")
	}
}
