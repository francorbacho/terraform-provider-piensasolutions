package client

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
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

// GenerateTOTP derives the current 6-digit TOTP code (RFC 6238, 30s step,
// SHA1) from a base32 secret, for non-interactive login. This is the only
// place in the repo that computes 2FA codes from a secret; the CLI's
// `login --2fa` flag/PIENSA_TOTP_SECRET env var and the Terraform
// provider's `totp_secret` config both go through this.
func GenerateTOTP(secret string) (string, error) {
	return generateTOTPAt(secret, time.Now())
}

func generateTOTPAt(secret string, t time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", fmt.Errorf("decode base32: %w", err)
	}
	counter := t.Unix() / 30
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

// Login sends username + password + 2FA to public-gateway.php and returns
// the session token (tkn) and HMAC key (pvtKey).
//
// The response is:
//  1. 302 redirect with tkn cookie (no pvtKey yet)
//  2. Following the redirect sets the pvtKey cookie
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

	jar := makeCookieJar()
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if Verbose {
				fmt.Printf("[verbose]   302 -> %s\n", r.URL.String())
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login request: %w", err)
	}
	resp.Body.Close()

	if Verbose {
		fmt.Printf("[verbose] Final response: HTTP %d\n", resp.StatusCode)
	}

	// Extract cookies accumulated across redirects
	allCookies := jar.AllCookies()
	if Verbose {
		for _, c := range allCookies {
			fmt.Printf("[verbose]   Cookie: %s=%s\n", c.Name, c.Value)
		}
	}

	var tkn, pvtKey string
	for _, c := range allCookies {
		switch c.Name {
		case "tkn":
			tkn = c.Value
		case "pvtKey":
			pvtKey = c.Value
		}
	}
	if tkn == "" {
		return nil, fmt.Errorf("login failed: no session token (tkn) in cookies")
	}
	if pvtKey == "" {
		return nil, fmt.Errorf("login failed: no pvtKey in cookies")
	}

	return &LoginResult{SessionToken: tkn, PvtKey: pvtKey}, nil
}

// simpleCookieJar collects cookies from responses.
type simpleCookieJar struct {
	cookies []*http.Cookie
}

func makeCookieJar() *simpleCookieJar {
	return &simpleCookieJar{}
}

func (j *simpleCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	for _, c := range cookies {
		existing := -1
		for i, e := range j.cookies {
			if e.Name == c.Name {
				existing = i
				break
			}
		}
		if existing >= 0 {
			j.cookies[existing] = c
		} else {
			j.cookies = append(j.cookies, c)
		}
	}
}

func (j *simpleCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return j.cookies
}

func (j *simpleCookieJar) AllCookies() []*http.Cookie {
	return j.cookies
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
		if Verbose {
			fmt.Printf("[verbose] VPS service: idsco=%d name=%q\n", svc.IDsco, svc.Des)
		}

		xsrfToken, ttl, err := PanellinkToXSRF(sc, svc.IDsco)
		if err != nil {
			if Verbose {
				fmt.Printf("[verbose]   panellink error: %v\n", err)
			}
			continue
		}

		vps := DiscoveredVPS{
			IDsco:     svc.IDsco,
			Name:      svc.Des,
			XSRFToken: xsrfToken,
			ExpiresAt: time.Now().Add(ttl),
		}

		c := New(xsrfToken)
		servers, err := DiscoverServers(c)
		if err != nil {
			if Verbose {
				fmt.Printf("[verbose]   discover error: %v\n", err)
			}
		} else if len(servers) > 0 {
			vps.ServerUUID = servers[0].ID
			vps.Name = servers[0].Name
			if Verbose {
				fmt.Printf("[verbose]   server UUID: %s name: %s\n", vps.ServerUUID[:8], vps.Name)
			}
		} else {
			if Verbose {
				fmt.Printf("[verbose]   discover returned 0 servers for this token\n")
			}
		}

		out = append(out, vps)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("could not generate any XSRF token")
	}
	return out, nil
}
