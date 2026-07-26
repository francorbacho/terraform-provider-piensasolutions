package provider

import (
	"context"
	"fmt"

	"github.com/fran/piensa/pkg/client"
	"github.com/fran/piensa/pkg/models"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func serverResource() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"power_state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"os_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"cpu": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"ram": {
				Type:     schema.TypeFloat,
				Computed: true,
			},
			"disk": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"ip_addresses": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
		CreateContext: serverCreate,
		ReadContext:   serverRead,
		DeleteContext: serverDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

func serverCreate(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	return diag.Errorf("piensa_server cannot be created, use 'terraform import piensa_server.<name> <server_id>'")
}

func serverRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	prov := m.(*piensaProvider)
	serverID := d.Id()

	c, diags := clientForServer(prov, serverID)
	if diags != nil {
		return diags
	}

	servers, err := client.DiscoverServers(c)
	if err != nil {
		return diag.FromErr(fmt.Errorf("read server %s: %w", serverID, err))
	}

	var found *models.Server
	for i := range servers {
		if servers[i].ID == serverID {
			found = &servers[i]
			break
		}
	}
	if found == nil {
		d.SetId("")
		return nil
	}

	d.Set("name", found.Name)
	d.Set("state", string(found.State))
	d.Set("power_state", string(found.PowerState))
	d.Set("os_name", found.OSName)
	d.Set("cpu", found.Resources.CPU)
	d.Set("ram", found.Resources.RAM)
	d.Set("disk", found.Resources.Disk)
	ips := make([]interface{}, len(found.IPs))
	for i, ip := range found.IPs {
		ips[i] = ip.Address
	}
	d.Set("ip_addresses", ips)

	return nil
}

func serverDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	d.SetId("")
	return nil
}
