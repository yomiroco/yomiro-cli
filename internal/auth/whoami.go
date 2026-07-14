package auth

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/platform"
)

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated user, tenant, and token prefix",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := credentials.New()
			if err != nil {
				return err
			}
			c, err := store.Load()
			if errors.Is(err, credentials.ErrNotFound) {
				return fmt.Errorf("not logged in — run `yomiro auth login`")
			}
			if err != nil {
				return err
			}
			// Credentials saved before the tenant was resolvable (or by a key that
			// couldn't read /organizations/me) carry an empty Tenant. Resolve it
			// live and heal the stored copy, rather than printing a blank field.
			tenant := c.Tenant
			if tenant == "" {
				if org, err := platform.New(c.APIURL, c.Token).CurrentOrganization(); err == nil && org != nil {
					tenant = org.Name
					c.Tenant = tenant
					_ = store.Save(c)
				}
			}

			out := struct {
				User   string `json:"user"`
				Tenant string `json:"tenant"`
				APIURL string `json:"api_url"`
				Prefix string `json:"token_prefix"`
			}{
				User:   c.User,
				Tenant: tenant,
				APIURL: c.APIURL,
				Prefix: prefixOf(c.Token),
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		},
	}
}

func prefixOf(token string) string {
	if len(token) < 12 {
		return token
	}
	return token[:12] + "…"
}
