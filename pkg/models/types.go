package models

import "time"

type PowerState string

const (
	PowerOn  PowerState = "ON"
	PowerOff PowerState = "OFF"
)

type ServerState string

const (
	StateActive       ServerState = "ACTIVE"
	StateConfiguring  ServerState = "CONFIGURING"
	StateSuspended    ServerState = "SUSPENDED"
	StateDisabled     ServerState = "DISABLED"
)

type Protocol string

const (
	ProtocolTCP Protocol = "TCP"
	ProtocolUDP Protocol = "UDP"
	ProtocolICMP Protocol = "ICMP"
)

type RuleAction string

const (
	RuleActionAllow RuleAction = "ALLOW"
	RuleActionDeny  RuleAction = "DENY"
)

type ServerResources struct {
	CPU  int     `json:"cpu"`
	RAM  float64 `json:"ram"`
	Disk int     `json:"disk"`
}

type IPAddress struct {
	ID         string `json:"id"`
	Address    string `json:"address"`
	Main       bool   `json:"main"`
	Type       string `json:"type"`
	ReverseDNS string `json:"reverse_dns,omitempty"`
}

type Server struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	State        ServerState     `json:"state"`
	PowerState   PowerState      `json:"power_state"`
	OSName       string          `json:"os_name"`
	OSType       string          `json:"os_type"`
	DatacenterID string          `json:"datacenter_id"`
	Resources    ServerResources `json:"resources"`
	IPs          []IPAddress     `json:"ips"`
}

type FirewallRule struct {
	ID          string     `json:"id"`
	Action      RuleAction `json:"action"`
	Protocol    Protocol   `json:"protocol"`
	PortFrom    int        `json:"port_from"`
	PortTo      int        `json:"port_to"`
	AllowedIP   string     `json:"allowed_ip"`
	Description string     `json:"description"`
}

type FirewallPolicy struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	State  ServerState    `json:"state"`
	Rules  []FirewallRule `json:"rules"`
	ServerID string       `json:"server_id,omitempty"`
}

type ServerToken struct {
	ServerID  string    `json:"server_id"`
	ServerName string   `json:"server_name"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type Account struct {
	NIF    string        `json:"nif,omitempty"`
	Servers []ServerToken `json:"servers"`
}

type Config struct {
	Accounts []Account `json:"accounts"`
}
