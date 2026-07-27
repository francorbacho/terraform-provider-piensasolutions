package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/fran/piensa/pkg/client"
	"github.com/fran/piensa/pkg/config"
	"github.com/fran/piensa/pkg/models"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"nif": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"password": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
			},
			"totp_secret": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
			},
		},
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
	nif := d.Get("nif").(string)
	password := d.Get("password").(string)
	totpSecret := d.Get("totp_secret").(string)

	if nif != "" && password != "" && totpSecret != "" {
		if cached, ok := loadCachedAccount(nif); ok {
			return &piensaProvider{cfg: &models.Config{Accounts: []models.Account{cached}}}, nil
		}

		code, err := generateTOTP(totpSecret)
		if err != nil {
			return nil, diag.FromErr(fmt.Errorf("generate totp: %w", err))
		}
		vps, err := client.FullLogin(client.LoginCredentials{
			NIF:      nif,
			Password: password,
			Code:     code,
		})
		if err != nil {
			return nil, diag.FromErr(fmt.Errorf("login: %w", err))
		}
		acc := models.Account{NIF: nif}
		for _, v := range vps {
			acc.Servers = append(acc.Servers, models.ServerToken{
				ServerID:   v.ServerUUID,
				ServerName: v.Name,
				Token:      v.XSRFToken,
				ExpiresAt:  v.ExpiresAt,
			})
		}
		cacheAccount(acc)
		return &piensaProvider{cfg: &models.Config{Accounts: []models.Account{acc}}}, nil
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, diag.FromErr(err)
	}
	if len(cfg.Accounts) == 0 || len(cfg.Accounts[0].Servers) == 0 {
		return nil, diag.Errorf("no servers configured. Provide nif/password/totp_secret in provider config or run 'piensa login'")
	}
	return &piensaProvider{cfg: cfg}, nil
}

// tokenRefreshBuffer avoids handing out a token that expires mid-plan.
const tokenRefreshBuffer = 2 * time.Minute

// loadCachedAccount returns a still-valid cached session for nif, if any,
// so repeated `tofu plan` runs within the XSRF token TTL (~55min) don't
// have to log in again.
func loadCachedAccount(nif string) (models.Account, bool) {
	cfg, err := config.Load()
	if err != nil {
		return models.Account{}, false
	}
	for _, acc := range cfg.Accounts {
		if acc.NIF != nif || len(acc.Servers) == 0 {
			continue
		}
		valid := true
		for _, st := range acc.Servers {
			if st.Token == "" || !st.ExpiresAt.After(time.Now().Add(tokenRefreshBuffer)) {
				valid = false
				break
			}
		}
		if valid {
			return acc, true
		}
	}
	return models.Account{}, false
}

// cacheAccount persists a freshly logged-in account for reuse by later
// provider runs, keeping any other accounts already in the config file.
func cacheAccount(acc models.Account) {
	cfg, err := config.Load()
	if err != nil {
		cfg = &models.Config{}
	}
	replaced := false
	for i := range cfg.Accounts {
		if cfg.Accounts[i].NIF == acc.NIF {
			cfg.Accounts[i] = acc
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.Accounts = append(cfg.Accounts, acc)
	}
	// Best-effort: failing to persist the cache shouldn't fail the provider.
	_ = config.Save(cfg)
}

func generateTOTP(secret string) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", fmt.Errorf("decode base32: %w", err)
	}
	counter := time.Now().Unix() / 30
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, uint64(counter))
	h := hmac.New(sha1.New, key)
	h.Write(msg)
	digest := h.Sum(nil)
	offset := digest[len(digest)-1] & 0x0F
	code := (int(digest[offset])&0x7F)<<24 |
		(int(digest[offset+1])&0xFF)<<16 |
		(int(digest[offset+2])&0xFF)<<8 |
		(int(digest[offset+3]) & 0xFF)
	code %= 1000000
	return fmt.Sprintf("%06d", code), nil
}

func clientForServer(prov *piensaProvider, serverID string) (*client.Client, diag.Diagnostics) {
	_, st := config.FindAccountByServerID(prov.cfg, serverID)
	if st == nil {
		return nil, diag.Errorf("no token found for server %s", serverID)
	}
	return client.New(st.Token), nil
}
