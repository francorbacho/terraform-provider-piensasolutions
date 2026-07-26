package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/fran/piensa/pkg/client"
	"github.com/fran/piensa/pkg/config"
	"github.com/fran/piensa/pkg/models"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var version string

var rootCmd = &cobra.Command{
	Use:     "piensa",
	Short:   "PiensaSolutions VPS manager",
	Long:    `Manage your PiensaSolutions VPS servers, ports, and firewall rules.`,
	Version: version,
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		client.Verbose = verboseFlag
	},
}

var tokenFlag string
var verboseFlag bool
var loginNIF string
var loginPassword string
var loginCode string

func init() {
	rootCmd.PersistentFlags().StringVarP(&tokenFlag, "token", "t", "", "X-TOKEN (overrides config)")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show request/response details")
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

// --- login ---

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in with your PiensaSolutions credentials",
	Long: `Log in with NIF, password, and 2FA code.

The CLI will:
   1. Authenticate with your credentials
   2. Discover your VPS servers
   3. Generate per-VPS access tokens
   4. Save them to config

Per-VPS tokens last ~1 hour. Run "piensa login" again to refresh.

Flags:
  --nif       NIF (omit for interactive prompt)
  --password  Password (omit for interactive prompt)
  --2fa       2FA code (omit for interactive prompt)`,
	Run: func(cmd *cobra.Command, args []string) {
		if loginNIF != "" && loginPassword != "" && loginCode != "" {
			loginNonInteractive()
		} else {
			loginInteractive(cmd)
		}
	},
}

func loginInteractive(cmd *cobra.Command) {
	fmt.Print("NIF: ")
	var nif string
	fmt.Scanln(&nif)
	nif = strings.TrimSpace(nif)
	if nif == "" {
		fmt.Fprintln(os.Stderr, "NIF required")
		os.Exit(1)
	}

	fmt.Print("Password: ")
	passBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nread password:", err)
		os.Exit(1)
	}
	password := strings.TrimSpace(string(passBytes))
	fmt.Println()

	fmt.Print("2FA code: ")
	var code string
	fmt.Scanln(&code)
	code = strings.TrimSpace(code)
	if code == "" {
		fmt.Fprintln(os.Stderr, "2FA code required")
		os.Exit(1)
	}

	fmt.Print("Authenticating... ")
	vps, err := client.FullLogin(client.LoginCredentials{
		NIF:      nif,
		Password: password,
		Code:     code,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAILED")
		fmt.Fprintf(os.Stderr, "login: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	cfg := loadConfig()
	acct := models.Account{NIF: nif}
	for _, v := range vps {
		acct.Servers = append(acct.Servers, models.ServerToken{
			ServerID:   v.ServerUUID,
			ServerName: v.Name,
			Token:      v.XSRFToken,
			ExpiresAt:  v.ExpiresAt,
		})
	}
	mergeAccount(cfg, &acct)
	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Logged in. %d VPS server(s) configured.\n", len(vps))
	fmt.Println("Tokens expire in ~1h. Run 'piensa login' to refresh.")
}

func loginNonInteractive() {
	vps, err := client.FullLogin(client.LoginCredentials{
		NIF:      loginNIF,
		Password: loginPassword,
		Code:     loginCode,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "login: %v\n", err)
		os.Exit(1)
	}

	cfg := loadConfig()
	acct := models.Account{NIF: loginNIF}
	for _, v := range vps {
		acct.Servers = append(acct.Servers, models.ServerToken{
			ServerID:   v.ServerUUID,
			ServerName: v.Name,
			Token:      v.XSRFToken,
			ExpiresAt:  v.ExpiresAt,
		})
	}
	mergeAccount(cfg, &acct)
	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Logged in. %d VPS server(s) configured.\n", len(vps))
}

func mergeAccount(cfg *models.Config, acct *models.Account) {
	if len(cfg.Accounts) == 0 {
		cfg.Accounts = append(cfg.Accounts, *acct)
		return
	}
	byID := make(map[string]*models.ServerToken)
	for i := range cfg.Accounts[0].Servers {
		byID[cfg.Accounts[0].Servers[i].ServerID] = &cfg.Accounts[0].Servers[i]
	}
	for _, st := range acct.Servers {
		if existing, ok := byID[st.ServerID]; ok {
			existing.Token = st.Token
			existing.ExpiresAt = st.ExpiresAt
			existing.ServerName = st.ServerName
		} else {
			cfg.Accounts[0].Servers = append(cfg.Accounts[0].Servers, st)
		}
	}
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
			if strings.Contains(err.Error(), "HTTP 403") || strings.Contains(err.Error(), "HTTP 401") {
				fmt.Fprintln(os.Stderr, "Tokens expired. Run: piensa login")
			} else {
				fmt.Fprintf(os.Stderr, "discover: %v\n", err)
			}
			os.Exit(1)
		}
		if len(all) == 0 {
			fmt.Println("No servers found.")
			return
		}
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

// --- firewall ---

var firewallCmd = &cobra.Command{
	Use:     "fw",
	Aliases: []string{"firewall"},
	Short:   "Manage firewall rules",
	Long: `Show, allow, and deny firewall rules.

Subcommands:
  show          List firewall rules for all servers (default)
  allow <srv> <port> [<protocol>]  Allow a port
  deny  <srv> <port> [<protocol>]  Deny/close a port

Run "piensa fw --help" for more details.`,
	Run: func(cmd *cobra.Command, args []string) {
		showFirewall(cmd, args)
	},
}

var fwShowCmd = &cobra.Command{
	Use:   "show",
	Short: "List firewall rules for all servers",
	Run:   showFirewall,
}

var fwAllowCmd = &cobra.Command{
	Use:   "allow <server> <port> [<protocol>]",
	Short: "Allow a port on a server",
	Args:  cobra.RangeArgs(2, 3),
	Run: func(cmd *cobra.Command, args []string) {
		serverID := args[0]
		portStr := args[1]
		protocol := "TCP"
		if len(args) > 2 {
			protocol = args[2]
		}
		description, _ := cmd.Flags().GetString("description")

		cfg := loadConfig()
		c := resolveClientForServer(cfg, serverID)
		policies, err := client.ListFirewallPolicies(c)
		if err != nil || len(policies) == 0 {
			fmt.Fprintln(os.Stderr, "no firewall policies for this server")
			os.Exit(1)
		}
		var p int
		fmt.Sscanf(portStr, "%d", &p)
		for _, pol := range policies {
			for _, r := range pol.Rules {
				if r.PortFrom == p && r.PortTo == p && strings.EqualFold(string(r.Protocol), protocol) {
					fmt.Fprintf(os.Stderr, "port %s/%s is already allowed on %s\n", portStr, protocol, serverID)
					os.Exit(1)
				}
			}
		}
		if err := client.OpenPort(c, policies[0].ID, p, protocol, description); err != nil {
			fmt.Fprintf(os.Stderr, "allow port: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Allowed %s/%s on %s\n", portStr, protocol, serverID)
	},
}

var fwDenyCmd = &cobra.Command{
	Use:   "deny <server> <port> [<protocol>]",
	Short: "Deny/close a port on a server",
	Args:  cobra.RangeArgs(2, 3),
	Run: func(cmd *cobra.Command, args []string) {
		serverID := args[0]
		portStr := args[1]
		protocol := "TCP"
		if len(args) > 2 {
			protocol = args[2]
		}

		cfg := loadConfig()
		c := resolveClientForServer(cfg, serverID)
		policies, err := client.ListFirewallPolicies(c)
		if err != nil || len(policies) == 0 {
			fmt.Fprintln(os.Stderr, "no firewall policies for this server")
			os.Exit(1)
		}
		var p int
		fmt.Sscanf(portStr, "%d", &p)
		for _, pol := range policies {
			for _, r := range pol.Rules {
				if r.PortFrom == p && r.PortTo == p && strings.EqualFold(string(r.Protocol), protocol) {
					if err := client.ClosePort(c, pol.ID, r.ID); err != nil {
						fmt.Fprintf(os.Stderr, "deny port: %v\n", err)
						os.Exit(1)
					}
					fmt.Printf("Denied %s/%s on %s (rule %s)\n",
						portStr, protocol, serverID, r.ID[:8])
					return
				}
			}
		}
		fmt.Fprintf(os.Stderr, "no allow rule found for port %s/%s on %s\n", portStr, protocol, serverID)
		os.Exit(1)
	},
}

func showFirewall(cmd *cobra.Command, args []string) {
	cfg := loadConfig()
	tokens := resolveTokens(cfg)
	all, tokenMap, err := client.DiscoverAllServers(tokens)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 403") || strings.Contains(err.Error(), "HTTP 401") {
			fmt.Fprintln(os.Stderr, "Tokens expired. Run: piensa login")
		} else {
			fmt.Fprintf(os.Stderr, "discover: %v\n", err)
		}
		os.Exit(1)
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
}

// --- restart / start / shutdown / suspend / resume ---

func makeActionCmd(use, short string, action string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <server-id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			inputID := args[0]
			cfg := loadConfig()
			_, st := config.FindAccountByServerID(cfg, inputID)
			if st == nil {
				fmt.Fprintf(os.Stderr, "no token for server %s\n", inputID)
				os.Exit(1)
			}
			c := client.New(st.Token)
			if _, err := client.RawServerAction(c, st.ServerID, action); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
				os.Exit(1)
			}
			fmt.Printf("%s initiated for %s\n", action, st.ServerID[:8])
		},
	}
}

func init() {
	rootCmd.AddCommand(loginCmd)
	loginCmd.Flags().StringVar(&loginNIF, "nif", "", "NIF for non-interactive login")
	loginCmd.Flags().StringVar(&loginPassword, "password", "", "Password for non-interactive login")
	loginCmd.Flags().StringVar(&loginCode, "2fa", "", "2FA code for non-interactive login")
	rootCmd.AddCommand(listCmd)

	fwAllowCmd.Flags().StringP("description", "d", "", "Description for the allow rule")
	firewallCmd.AddCommand(fwShowCmd)
	firewallCmd.AddCommand(fwAllowCmd)
	firewallCmd.AddCommand(fwDenyCmd)
	rootCmd.AddCommand(firewallCmd)

	rootCmd.AddCommand(makeActionCmd("restart", "Restart a server", "reboot"))
	rootCmd.AddCommand(makeActionCmd("start", "Start a server", "start"))
	rootCmd.AddCommand(makeActionCmd("shutdown", "Shutdown a server", "shutdown"))
	rootCmd.AddCommand(makeActionCmd("suspend", "Suspend a server", "suspend"))
	rootCmd.AddCommand(makeActionCmd("resume", "Resume a server", "resume"))
}
