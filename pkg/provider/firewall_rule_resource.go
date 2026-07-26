package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/fran/piensa/pkg/client"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func firewallRuleResource() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"server_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"protocol": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"port": {
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"action": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"allowed_ip": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
		CreateContext: firewallRuleCreate,
		ReadContext:   firewallRuleRead,
		DeleteContext: firewallRuleDelete,
		UpdateContext: firewallRuleUpdate,
		Importer: &schema.ResourceImporter{
			StateContext: firewallRuleImport,
		},
	}
}

func importID(id string) (serverID, protocol string, port int, err error) {
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 {
		return "", "", 0, fmt.Errorf("invalid import ID %q, expected <server_id>:<port>:<protocol>", id)
	}
	serverID = parts[0]
	port, err = strconv.Atoi(parts[1])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid port in import ID %q: %w", id, err)
	}
	protocol = parts[2]
	return
}

func ruleID(serverID, protocol string, port int) string {
	return fmt.Sprintf("%s:%d:%s", serverID, port, protocol)
}

func findRule(c *client.Client, port int, protocol string) (ruleUUID, policyID, allowedIP string, err error) {
	policies, err := client.ListFirewallPolicies(c)
	if err != nil {
		return "", "", "", err
	}
	for _, p := range policies {
		for _, r := range p.Rules {
			if r.PortFrom == port && r.PortTo == port && strings.EqualFold(string(r.Protocol), protocol) {
				return r.ID, p.ID, string(r.AllowedIP), nil
			}
		}
	}
	return "", "", "", nil
}

func firewallRuleCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	prov := m.(*piensaProvider)
	serverID := d.Get("server_id").(string)
	protocol := d.Get("protocol").(string)
	port := d.Get("port").(int)
	description := d.Get("description").(string)

	c, diags := clientForServer(prov, serverID)
	if diags != nil {
		return diags
	}

	policies, err := client.ListFirewallPolicies(c)
	if err != nil || len(policies) == 0 {
		return diag.Errorf("no firewall policies for server %s", serverID)
	}

	if err := client.OpenPort(c, policies[0].ID, port, protocol, description); err != nil {
		return diag.FromErr(fmt.Errorf("create firewall rule: %w", err))
	}

	d.SetId(ruleID(serverID, protocol, port))
	return firewallRuleRead(ctx, d, m)
}

func firewallRuleRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	prov := m.(*piensaProvider)
	serverID, protocol, port, err := importID(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	c, diags := clientForServer(prov, serverID)
	if diags != nil {
		return diags
	}

	ruleUUID, _, allowedIP, err := findRule(c, port, protocol)
	if err != nil {
		return diag.FromErr(fmt.Errorf("read firewall rule: %w", err))
	}
	if ruleUUID == "" {
		d.SetId("")
		return nil
	}

	d.Set("server_id", serverID)
	d.Set("protocol", protocol)
	d.Set("port", port)
	d.Set("action", "ALLOW")
	d.Set("allowed_ip", allowedIP)
	return nil
}

func firewallRuleDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	prov := m.(*piensaProvider)
	serverID, protocol, port, err := importID(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	c, diags := clientForServer(prov, serverID)
	if diags != nil {
		return diags
	}

	ruleUUID, policyID, _, err := findRule(c, port, protocol)
	if err != nil {
		return diag.FromErr(fmt.Errorf("delete firewall rule (find): %w", err))
	}
	if ruleUUID == "" {
		d.SetId("")
		return nil
	}

	if err := client.ClosePort(c, policyID, ruleUUID); err != nil {
		return diag.FromErr(fmt.Errorf("delete firewall rule: %w", err))
	}

	d.SetId("")
	return nil
}

func firewallRuleUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	return diag.Errorf("firewall rule attributes are immutable, recreate with 'terraform taint'")
}

func firewallRuleImport(_ context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	serverID, protocol, port, err := importID(d.Id())
	if err != nil {
		return nil, err
	}
	d.Set("server_id", serverID)
	d.Set("protocol", protocol)
	d.Set("port", port)
	return []*schema.ResourceData{d}, nil
}
