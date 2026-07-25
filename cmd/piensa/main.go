package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fran/piensa/pkg/client"
	"github.com/fran/piensa/pkg/config"
	"github.com/fran/piensa/pkg/models"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "piensa",
	Short: "PiensaSolutions VPS manager",
	Long: `Manage your PiensaSolutions VPS servers, ports, and firewall rules.

Config is stored at ~/.config/piensa/config.json.
Use "piensa login" to set up your tokens.`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

var tokenFlag string

func init() {
	rootCmd.PersistentFlags().StringVarP(&tokenFlag, "token", "t", "", "X-TOKEN (overrides config)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// --- helpers ---

func loadConfig() *models.Config {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		return &models.Config{}
	}
	return cfg
}

func resolveTokens(cfg *models.Config) []string {
	if tokenFlag != "" {
		return []string{tokenFlag}
	}
	var tokens []string
	for _, acct := range cfg.Accounts {
		for _, st := range acct.Servers {
			if st.Token != "" {
				tokens = append(tokens, st.Token)
			}
		}
	}
	if len(tokens) == 0 {
		fmt.Fprintln(os.Stderr, "no tokens found. Run: piensa login")
		os.Exit(1)
	}
	return tokens
}

func resolveClientForServer(cfg *models.Config, serverID string) *client.Client {
	if tokenFlag != "" {
		return client.New(tokenFlag)
	}
	_, st := config.FindAccountByServerID(cfg, serverID)
	if st == nil || st.Token == "" {
		fmt.Fprintf(os.Stderr, "no token for server %s\n", serverID)
		os.Exit(1)
	}
	return client.New(st.Token)
}

func printJSON(v interface{}) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

// --- login ---

var loginCmd = &cobra.Command{
	Use:   "login [token]",
	Short: "Add a front-cloudpanel X-TOKEN",
	Long: `Store one or more X-TOKENs for accessing the CoreVPS API.

You can get tokens from:
  1. Open https://cloudpanel.piensasolutions.com in your browser
  2. Open DevTools (F12) → Network tab
  3. Look for requests to front-cloudpanel.piensasolutions.com
  4. Copy the X-TOKEN header value

Pass multiple tokens separated by commas to register all your VPS.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		raw := ""
		if len(args) > 0 {
			raw = args[0]
		} else {
			fmt.Print("Paste X-TOKEN(s) (comma-separated for multiple VPS): ")
			fmt.Scanln(&raw)
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			fmt.Fprintln(os.Stderr, "no token provided")
			os.Exit(1)
		}
		tokens := strings.Split(raw, ",")
		for i := range tokens {
			tokens[i] = strings.TrimSpace(tokens[i])
		}

		// Discover servers for each token
		all, tokenMap, err := client.DiscoverAllServers(tokens)
		if err != nil {
			fmt.Fprintf(os.Stderr, "discovery: %v\n", err)
			os.Exit(1)
		}

		cfg := loadConfig()
		acct := models.Account{}
		for _, s := range all {
			tok := tokenMap[s.ID]
			st := models.ServerToken{
				ServerID:   s.ID,
				ServerName: s.Name,
				Token:      tok,
			}
			acct.Servers = append(acct.Servers, st)
		}
		if len(cfg.Accounts) == 0 {
			cfg.Accounts = append(cfg.Accounts, acct)
		} else {
			// Merge into first account (dedup by server ID)
			existing := make(map[string]bool)
			for _, st := range cfg.Accounts[0].Servers {
				existing[st.ServerID] = true
			}
			for _, st := range acct.Servers {
				if !existing[st.ServerID] {
					cfg.Accounts[0].Servers = append(cfg.Accounts[0].Servers, st)
					existing[st.ServerID] = true
				}
			}
		}
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "save config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Saved %d server(s) to config.\n", len(all))
	},
}

// --- list ---

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all VPS servers",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		tokens := resolveTokens(cfg)
		all, _, err := client.DiscoverAllServers(tokens)
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: %v\n", err)
			os.Exit(1)
		}
		if len(all) == 0 {
			fmt.Println("No servers found.")
			return
		}
		// Print table
		fmt.Printf("%-12s %-20s %-10s %-8s %-18s %-5s %-6s %-6s %-16s\n",
			"SERVER ID", "NAME", "STATE", "POWER", "OS", "CPU", "RAM", "DISK", "IP")
		fmt.Println(strings.Repeat("-", 110))
		for _, s := range all {
			mainIP := "-"
			for _, ip := range s.IPs {
				if ip.Main {
					mainIP = ip.Address
					break
				}
			}
			if mainIP == "-" && len(s.IPs) > 0 {
				mainIP = s.IPs[0].Address
			}
			shortID := s.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			fmt.Printf("%-12s %-20s %-10s %-8s %-18s %-5d %-6.1f %-6d %-16s\n",
				shortID, s.Name, s.State, s.PowerState, s.OSName,
				s.Resources.CPU, s.Resources.RAM, s.Resources.Disk, mainIP)
		}
	},
}

// --- ports ---

var portsCmd = &cobra.Command{
	Use:   "ports",
	Short: "List firewall rules for all servers",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		tokens := resolveTokens(cfg)
		all, tokenMap, err := client.DiscoverAllServers(tokens)
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: %v\n", err)
			os.Exit(1)
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")

		if jsonFlag {
			type policyWithServer struct {
				Policy   models.FirewallPolicy `json:"policy"`
				ServerID string                `json:"server_id"`
				Server   string                `json:"server_name"`
			}
			var result []policyWithServer
			for _, s := range all {
				c := client.New(tokenMap[s.ID])
				policies, err := client.ListFirewallPolicies(c)
				if err != nil {
					continue
				}
				for _, p := range policies {
					p.ServerID = s.ID
					result = append(result, policyWithServer{Policy: p, ServerID: s.ID, Server: s.Name})
				}
			}
			printJSON(result)
			return
		}

		for _, s := range all {
			c := client.New(tokenMap[s.ID])
			policies, err := client.ListFirewallPolicies(c)
			if err != nil {
				fmt.Fprintf(os.Stderr, "firewall for %s: %v\n", s.Name, err)
				continue
			}
			for _, p := range policies {
				fmt.Printf("\nServer: %s  |  Policy: %s (%s)  [%s]\n", s.Name, p.Name, p.ID[:8], p.State)
				if len(p.Rules) == 0 {
					fmt.Println("  (no rules)")
					continue
				}
				fmt.Printf("  %-12s %-8s %-6s %-12s %-12s  %s\n",
					"RULE ID", "ACTION", "PROTO", "PORT", "ALLOWED IP", "DESCRIPTION")
				fmt.Printf("  %s\n", strings.Repeat("-", 80))
				for _, r := range p.Rules {
					portStr := fmt.Sprintf("%d", r.PortFrom)
					if r.PortTo != r.PortFrom {
						portStr = fmt.Sprintf("%d-%d", r.PortFrom, r.PortTo)
					}
					shortID := r.ID[:8]
					fmt.Printf("  %-12s %-8s %-6s %-12s %-12s  %s\n",
						shortID, r.Action, r.Protocol, portStr, r.AllowedIP, r.Description)
				}
			}
		}
	},
}

// --- open ---

var openCmd = &cobra.Command{
	Use:   "open <port>",
	Short: "Open a port in the firewall",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		port := args[0]
		protocol, _ := cmd.Flags().GetString("protocol")
		description, _ := cmd.Flags().GetString("description")
		serverID, _ := cmd.Flags().GetString("server")

		cfg := loadConfig()
		tokens := resolveTokens(cfg)

		if serverID != "" {
			c := resolveClientForServer(cfg, serverID)
			policies, err := client.ListFirewallPolicies(c)
			if err != nil || len(policies) == 0 {
				fmt.Fprintln(os.Stderr, "no firewall policies for this server")
				os.Exit(1)
			}
			// Parse port
			var p int
			fmt.Sscanf(port, "%d", &p)
			if err := client.OpenPort(c, policies[0].ID, p, protocol, description); err != nil {
				fmt.Fprintf(os.Stderr, "open port: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Opened port %s/%s on %s\n", port, protocol, serverID)
			return
		}

		// No server specified: apply to all
		all, tokenMap, err := client.DiscoverAllServers(tokens)
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: %v\n", err)
			os.Exit(1)
		}
		var p int
		fmt.Sscanf(port, "%d", &p)
		for _, s := range all {
			c := client.New(tokenMap[s.ID])
			policies, err := client.ListFirewallPolicies(c)
			if err != nil || len(policies) == 0 {
				continue
			}
			if err := client.OpenPort(c, policies[0].ID, p, protocol, description); err != nil {
				fmt.Fprintf(os.Stderr, "open port on %s: %v\n", s.Name, err)
				continue
			}
			fmt.Printf("Opened port %s/%s on %s (%s)\n", port, protocol, s.Name, s.ID[:8])
		}
	},
}

// --- close ---

var closeCmd = &cobra.Command{
	Use:   "close <rule-id>",
	Short: "Close a firewall port by rule ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ruleID := args[0]
		serverID, _ := cmd.Flags().GetString("server")
		cfg := loadConfig()

		// If server specified, close there
		if serverID != "" {
			c := resolveClientForServer(cfg, serverID)
			policies, err := client.ListFirewallPolicies(c)
			if err != nil || len(policies) == 0 {
				fmt.Fprintln(os.Stderr, "no firewall policies")
				os.Exit(1)
			}
			for _, p := range policies {
				for _, r := range p.Rules {
					if strings.HasPrefix(r.ID, ruleID) || r.ID == ruleID {
						if err := client.ClosePort(c, p.ID, r.ID); err != nil {
							fmt.Fprintf(os.Stderr, "close: %v\n", err)
							os.Exit(1)
						}
						fmt.Printf("Closed rule %s (%s/%d) on %s\n",
							r.ID[:8], r.Protocol, r.PortFrom, serverID)
						return
					}
				}
			}
			fmt.Fprintf(os.Stderr, "rule %s not found\n", ruleID)
			os.Exit(1)
		}

		// Search all servers
		tokens := resolveTokens(cfg)
		all, tokenMap, err := client.DiscoverAllServers(tokens)
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: %v\n", err)
			os.Exit(1)
		}
		for _, s := range all {
			c := client.New(tokenMap[s.ID])
			policies, err := client.ListFirewallPolicies(c)
			if err != nil {
				continue
			}
			for _, p := range policies {
				for _, r := range p.Rules {
					if strings.HasPrefix(r.ID, ruleID) || r.ID == ruleID {
						if err := client.ClosePort(c, p.ID, r.ID); err != nil {
							fmt.Fprintf(os.Stderr, "close on %s: %v\n", s.Name, err)
							continue
						}
						fmt.Printf("Closed rule %s (%s/%d) on %s (%s)\n",
							r.ID[:8], r.Protocol, r.PortFrom, s.Name, s.ID[:8])
						return
					}
				}
			}
		}
		fmt.Fprintf(os.Stderr, "rule %s not found on any server\n", ruleID)
		os.Exit(1)
	},
}

// --- restart / start / shutdown / suspend / resume ---

func makeActionCmd(use, short string, action string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <server-id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			serverID := args[0]
			cfg := loadConfig()
			c := resolveClientForServer(cfg, serverID)
			if _, err := client.RawServerAction(c, serverID, action); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
				os.Exit(1)
			}
			fmt.Printf("%s initiated for %s\n", action, serverID[:8])
		},
	}
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(listCmd)

	portsCmd.Flags().BoolP("json", "j", false, "JSON output")
	rootCmd.AddCommand(portsCmd)

	openCmd.Flags().StringP("protocol", "p", "TCP", "Protocol (TCP, UDP)")
	openCmd.Flags().StringP("description", "d", "", "Description")
	openCmd.Flags().StringP("server", "s", "", "Server ID (apply to all if empty)")
	rootCmd.AddCommand(openCmd)

	closeCmd.Flags().StringP("server", "s", "", "Server ID (search all if empty)")
	rootCmd.AddCommand(closeCmd)

	rootCmd.AddCommand(makeActionCmd("restart", "Restart a server", "reboot"))
	rootCmd.AddCommand(makeActionCmd("start", "Start a server", "start"))
	rootCmd.AddCommand(makeActionCmd("shutdown", "Shutdown a server", "shutdown"))
	rootCmd.AddCommand(makeActionCmd("suspend", "Suspend a server", "suspend"))
	rootCmd.AddCommand(makeActionCmd("resume", "Resume a server", "resume"))
}
