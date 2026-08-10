package client

import (
	"encoding/json"
	"fmt"

	"github.com/francorbacho/terraform-provider-piensasolutions/pkg/models"
)

type firewallPolicyResponse struct {
	Items []firewallPolicyItem `json:"items"`
}

type firewallPolicyItem struct {
	ID         string                   `json:"id"`
	Properties firewallPolicyProperties `json:"properties"`
	Related    firewallRelated          `json:"related_entities"`
}

type firewallPolicyProperties struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type firewallRelated struct {
	Rules []firewallRuleItem `json:"firewall_policy_rule"`
	IPs   []ipItem           `json:"ip"`
}

type firewallRuleItem struct {
	ID         string               `json:"id"`
	Properties firewallRuleProperties `json:"properties"`
}

type firewallRuleProperties struct {
	Action      string `json:"action"`
	Protocol    string `json:"protocol"`
	PortFrom    int    `json:"port_from"`
	PortTo      int    `json:"port_to"`
	AllowedIP   string `json:"allowed_ip"`
	Description string `json:"description"`
}

// ListFirewallPolicies fetches all firewall policies visible to the client.
func ListFirewallPolicies(c *Client) ([]models.FirewallPolicy, error) {
	resp, err := c.get(FrontPanelBase + "/pss/firewall-policies?depth=3")
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	var fpr firewallPolicyResponse
	if err := json.Unmarshal(body, &fpr); err != nil {
		return nil, fmt.Errorf("parse firewall: %w", err)
	}
	var out []models.FirewallPolicy
	for _, item := range fpr.Items {
		p := models.FirewallPolicy{
			ID:    item.ID,
			Name:  item.Properties.Name,
			State: models.ServerState(item.Properties.State),
		}
		for _, r := range item.Related.Rules {
			p.Rules = append(p.Rules, models.FirewallRule{
				ID:          r.ID,
				Action:      models.RuleAction(r.Properties.Action),
				Protocol:    models.Protocol(r.Properties.Protocol),
				PortFrom:    r.Properties.PortFrom,
				PortTo:      r.Properties.PortTo,
				AllowedIP:   r.Properties.AllowedIP,
				Description: r.Properties.Description,
			})
		}
		out = append(out, p)
	}
	return out, nil
}

// OpenPort creates a new ALLOW rule for a single port.
func OpenPort(c *Client, policyID string, port int, protocol string, description string) error {
	body := map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"action":      "ALLOW",
				"allowed_ip":  "all",
				"protocol":    protocol,
				"port_from":   port,
				"port_to":     port,
				"description": description,
			},
		},
	}
	url := fmt.Sprintf("%s/pss/firewall-policies/%s/rules", FrontPanelBase, policyID)
	resp, err := c.post(url, body)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}

// ClosePort deletes a firewall rule by ID.
func ClosePort(c *Client, policyID, ruleID string) error {
	url := fmt.Sprintf("%s/pss/firewall-policies/%s/rules/%s", FrontPanelBase, policyID, ruleID)
	resp, err := c.del(url)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}


