package client

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type LoginCredentials struct {
	NIF      string
	Password string
	Code     string
}

type LoginResult struct {
	SessionToken string
	PvtKey       string
}

var Verbose bool

// Login sends username + password + 2FA to public-gateway.php and returns
// the session token (tkn) and HMAC key (pvtKey) from Set-Cookie headers.
func Login(creds LoginCredentials) (*LoginResult, error) {
	form := url.Values{
		"DAFLOGIN":      {creds.NIF},
		"DAFPASS":       {creds.Password},
		"DAFCODE":       {creds.Code},
		"gtw":           {"login"},
		"redirect":      {"https://www.piensasolutions.com/clientes?response=ok"},
		"redirect_fail": {"https://www.piensasolutions.com/clientes?response=ko"},
	}

	urlStr := GatewayURL
	req, err := http.NewRequest("POST", urlStr,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://www.piensasolutions.com")
	req.Header.Set("Referer", "https://www.piensasolutions.com/clientes")
	req.Header.Set("User-Agent", "piensa-cli/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	if Verbose {
		fmt.Printf("[verbose] POST %s\n", urlStr)
		fmt.Printf("[verbose] Content-Type: application/x-www-form-urlencoded\n")
		fmt.Printf("[verbose] Body: %s\n", form.Encode())
	}

	client := &http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if Verbose {
				fmt.Printf("[verbose] * NOT following redirect to: %s\n", r.URL.String())
			}
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if Verbose {
		fmt.Printf("[verbose] Response: HTTP %d\n", resp.StatusCode)
		for _, c := range resp.Cookies() {
			fmt.Printf("[verbose]   Cookie: %s=%s\n", c.Name, c.Value)
		}
	}

	if resp.StatusCode != 302 && resp.StatusCode != 200 {
		return nil, fmt.Errorf("login failed (HTTP %d)", resp.StatusCode)
	}

	var tkn, pvtKey string
	for _, c := range resp.Cookies() {
		switch c.Name {
		case "tkn":
			tkn = c.Value
		case "pvtKey":
			pvtKey = c.Value
		}
	}
	if tkn == "" {
		return nil, fmt.Errorf("login failed: no session token (tkn) in response cookies")
	}
	if pvtKey == "" {
		return nil, fmt.Errorf("login failed: no pvtKey in response cookies")
	}

	return &LoginResult{SessionToken: tkn, PvtKey: pvtKey}, nil
}

// --- Full login chain ---

type DiscoveredVPS struct {
	IDsco      int
	Name       string
	XSRFToken  string
	ServerUUID string
	ExpiresAt  time.Time
}

// FullLogin performs the complete auth chain:
// 1. POST to public-gateway.php → get session token + pvtKey
// 2. Call service/list → discover VPS IDs
// 3. For each VPS, call panellink → loginuser.php → extract XSRF token
// 4. Validate each XSRF token against the cloudpanel API
func FullLogin(creds LoginCredentials) ([]DiscoveredVPS, error) {
	loginResult, err := Login(creds)
	if err != nil {
		return nil, err
	}

	sc := NewSecure(loginResult.SessionToken, loginResult.PvtKey)

	services, err := DiscoverServiceIDs(sc)
	if err != nil {
		return nil, fmt.Errorf("discover services: %w", err)
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("no VPS services found in your account")
	}

	var out []DiscoveredVPS
	for _, svc := range services {
		xsrfToken, ttl, err := PanellinkToXSRF(sc, svc.IDsco)
		if err != nil {
			continue
		}

		vps := DiscoveredVPS{
			IDsco:     svc.IDsco,
			Name:      svc.Des,
			XSRFToken: xsrfToken,
			ExpiresAt: time.Now().Add(ttl),
		}

		c := New(xsrfToken)
		if servers, err := DiscoverServers(c); err == nil && len(servers) > 0 {
			vps.ServerUUID = servers[0].ID
			vps.Name = servers[0].Name
		}

		out = append(out, vps)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("could not generate any XSRF token")
	}
	return out, nil
}
