// Package platform wires the generated platform commands and the override
// registry into the CLI's root command.
package platform

import (
	"context"
	"net/http"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/platform/client"
	"github.com/yomiroco/yomiro-cli/internal/platform/generated"
	"github.com/yomiroco/yomiro-cli/internal/platform/overrides"
	"github.com/spf13/cobra"
)

// AddTo registers the generated platform commands and their overrides under
// the given root command. It is best-effort: a missing/empty credentials
// store still produces a usable command tree (with an empty bearer token),
// so help output works even before `yomiro auth login`.
func AddTo(root *cobra.Command) error {
	store, err := credentials.New()
	if err != nil {
		return err
	}

	apiURL := "https://api.yomiro.io"
	token := ""
	if c, err := store.Load(); err == nil {
		if c.APIURL != "" {
			apiURL = c.APIURL
		}
		token = c.Token
	}

	apiClient, err := client.NewClientWithResponses(apiURL, client.WithRequestEditorFn(bearer(token)))
	if err != nil {
		return err
	}

	// Skill has no matching generated command (only AgentSkills/AgentSkillFiles
	// exist, neither of which fits `yomiro skill install/publish`). Stub a
	// placeholder root so Task 6's overrides have a home.
	skillCmd := &cobra.Command{Use: "skill", Short: "Manage skills"}
	root.AddCommand(skillCmd)

	dash := generated.NewDashboardsCmd(apiClient)
	dash.Use = "dashboard"
	root.AddCommand(dash)

	capCmd := generated.NewCapturesCmd(apiClient)
	capCmd.Use = "capture"
	root.AddCommand(capCmd)

	inc := generated.NewIncidentsCmd(apiClient)
	inc.Use = "incident"
	root.AddCommand(inc)

	dev := generated.NewDevicesCmd(apiClient)
	dev.Use = "device"
	root.AddCommand(dev)

	for _, group := range []string{"skill", "dashboard", "capture", "incident", "device"} {
		groupCmd, _, err := root.Find([]string{group})
		if err != nil || groupCmd == nil || groupCmd == root {
			continue
		}
		for _, ov := range overrides.AllInGroup(group) {
			existing, _, _ := groupCmd.Find([]string{ov.Name()})
			if existing != nil && existing != groupCmd {
				groupCmd.RemoveCommand(existing)
			}
			groupCmd.AddCommand(ov)
		}
	}
	return nil
}

// bearer returns a RequestEditorFn that attaches a Bearer token to outbound
// requests. An empty token is a no-op so help/build flows work pre-login.
func bearer(token string) client.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return nil
	}
}
