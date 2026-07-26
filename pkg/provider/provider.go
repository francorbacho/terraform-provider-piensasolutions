package provider

import (
	"context"

	"github.com/fran/piensa/pkg/client"
	"github.com/fran/piensa/pkg/config"
	"github.com/fran/piensa/pkg/models"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"piensa_server":        serverResource(),
			"piensa_firewall_rule": firewallRuleResource(),
		},
		ConfigureContextFunc: configureProvider,
	}
}

type piensaProvider struct {
	cfg *models.Config
}

func configureProvider(_ context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	cfg, err := config.Load()
	if err != nil {
		return nil, diag.FromErr(err)
	}
	if len(cfg.Accounts) == 0 || len(cfg.Accounts[0].Servers) == 0 {
		return nil, diag.Errorf("no servers configured. Run 'piensa login' first")
	}
	return &piensaProvider{cfg: cfg}, nil
}

func clientForServer(prov *piensaProvider, serverID string) (*client.Client, diag.Diagnostics) {
	_, st := config.FindAccountByServerID(prov.cfg, serverID)
	if st == nil {
		return nil, diag.Errorf("no token found for server %s", serverID)
	}
	return client.New(st.Token), nil
}
