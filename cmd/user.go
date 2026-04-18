package cmd

import (
	"fmt"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type UserCmd struct {
	List UserListCmd `cmd:"" help:"List users in the workspace"`
	Info UserInfoCmd `cmd:"" help:"Show user information"`
}

type UserListCmd struct {
	Limit int  `help:"Maximum number of users to list" default:"100"`
	JSON  bool `help:"Output as pretty JSON array" short:"j" xor:"format"`
	JSONL bool `help:"Output as JSON Lines, one user per line" xor:"format"`
}

func (c *UserListCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}
	resp, err := client.ListUsers(c.Limit)
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	if c.JSONL {
		i := 0
		return output.EmitJSONLStream(func() (output.User, bool, error) {
			for i < len(resp.Members) {
				u := resp.Members[i]
				i++
				if u.Deleted || u.IsBot {
					continue
				}
				return output.ToUser(u), true, nil
			}
			return output.User{}, false, nil
		})
	}

	if c.JSON {
		records := make([]output.User, 0, len(resp.Members))
		for _, user := range resp.Members {
			if user.Deleted || user.IsBot {
				continue
			}
			records = append(records, output.ToUser(user))
		}
		return output.EmitJSON(records)
	}

	for _, user := range resp.Members {
		if user.Deleted || user.IsBot {
			continue
		}
		name := user.RealName
		if name == "" {
			name = user.Name
		}
		fmt.Printf("@%s - %s (%s)\n", user.Name, name, user.Profile.Title)
	}

	return nil
}

type UserInfoCmd struct {
	User  string `arg:"" help:"User ID or email"`
	JSON  bool   `help:"Output as pretty JSON object" short:"j" xor:"format"`
	JSONL bool   `help:"Output as a single JSON Lines record" xor:"format"`
}

func (c *UserInfoCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	var user *slack.User

	// Check if it looks like an email
	if len(c.User) > 0 && c.User[0] != 'U' && contains(c.User, "@") {
		user, err = client.LookupUserByEmail(c.User)
	} else {
		user, err = client.GetUserInfo(c.User)
	}

	if err != nil {
		return fmt.Errorf("failed to get user info: %w", err)
	}

	rec := output.ToUser(*user)
	if c.JSONL {
		return output.EmitJSONL([]output.User{rec})
	}
	if c.JSON {
		return output.EmitJSON(rec)
	}

	fmt.Printf("Name: %s\n", user.RealName)
	fmt.Printf("Username: @%s\n", user.Name)
	fmt.Printf("ID: %s\n", user.ID)
	if user.Profile.Title != "" {
		fmt.Printf("Title: %s\n", user.Profile.Title)
	}
	if user.Profile.Email != "" {
		fmt.Printf("Email: %s\n", user.Profile.Email)
	}
	if user.TZ != "" {
		fmt.Printf("Timezone: %s\n", user.TZ)
	}

	return nil
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
