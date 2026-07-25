package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fran/piensa/pkg/models"
)

type serverItem struct {
	ID         string           `json:"id"`
	Properties serverProperties `json:"properties"`
	Related    relatedEntities  `json:"related_entities"`
}

type serverProperties struct {
	Name         string                 `json:"name"`
	State        string                 `json:"state"`
	PowerState   string                 `json:"power_state"`
	OSName       string                 `json:"os_name"`
	OSType       string                 `json:"os_type"`
	DatacenterID string                 `json:"datacenter_id"`
	Resources    models.ServerResources `json:"resources"`
}

type relatedEntities struct {
	IPs []ipItem `json:"ip"`
}

type ipItem struct {
	ID         string       `json:"id"`
	Properties ipProperties `json:"properties"`
}

type ipProperties struct {
	Address    string `json:"address"`
	Main       bool   `json:"main"`
	Type       string `json:"type"`
	ReverseDNS string `json:"reverse_dns"`
}

type serversResponse struct {
	Items      []serverItem `json:"items"`
	PrevOffset *int         `json:"prev_offset"`
	NextOffset *int         `json:"next_offset"`
}

// --- CloudPanel API (front-cloudpanel) ---

func DiscoverServers(c *Client) ([]models.Server, error) {
	resp, err := c.get(FrontPanelBase + "/pss/servers?depth=3")
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
	var sr serversResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("parse servers: %w", err)
	}
	var out []models.Server
	for _, item := range sr.Items {
		s := models.Server{
			ID:           item.ID,
			Name:         item.Properties.Name,
			State:        models.ServerState(item.Properties.State),
			PowerState:   models.PowerState(item.Properties.PowerState),
			OSName:       item.Properties.OSName,
			OSType:       item.Properties.OSType,
			DatacenterID: item.Properties.DatacenterID,
			Resources:    item.Properties.Resources,

		}
		for _, ip := range item.Related.IPs {
			s.IPs = append(s.IPs, models.IPAddress{
				ID:         ip.ID,
				Address:    ip.Properties.Address,
				Main:       ip.Properties.Main,
				Type:       ip.Properties.Type,
				ReverseDNS: ip.Properties.ReverseDNS,
			})
		}
		out = append(out, s)
	}
	return out, nil
}

func DiscoverAllServers(tokens []string) ([]models.Server, map[string]string, error) {
	var all []models.Server
	tokenMap := make(map[string]string)
	seen := make(map[string]bool)

	if Verbose {
		fmt.Printf("[verbose] DiscoverAllServers: %d tokens\n", len(tokens))
	}

	for _, tok := range tokens {
		if tok == "" {
			if Verbose {
				fmt.Printf("[verbose]   skipping empty token\n")
			}
			continue
		}
		if Verbose {
			fmt.Printf("[verbose]   trying token %s...\n", tok[:12]+"...")
		}
		c := New(tok)
		servers, err := DiscoverServers(c)
		if err != nil {
			if Verbose {
				fmt.Printf("[verbose]     error: %v\n", err)
			}
			continue
		}
		if Verbose {
			fmt.Printf("[verbose]     returned %d server(s)\n", len(servers))
		}
		for _, s := range servers {
			if !seen[s.ID] {
				seen[s.ID] = true
				all = append(all, s)
				tokenMap[s.ID] = tok
				if Verbose {
					fmt.Printf("[verbose]       + %s %s\n", s.ID[:8], s.Name)
				}
			} else {
				if Verbose {
					fmt.Printf("[verbose]       - %s %s (duplicate)\n", s.ID[:8], s.Name)
				}
			}
		}
	}
	return all, tokenMap, nil
}

// --- Secure Panel API (secure.piensasolutions.com proxy.php with HMAC) ---

type servicioItem struct {
	IDsco int    `json:"idsco"`
	Des   string `json:"des"`
	URL   string `json:"url"`
	IDS   int    `json:"ids"`
	Log   int64  `json:"log"`
}

type familiaItem struct {
	IdFamilia int            `json:"IdFamilia"`
	Servicios []servicioItem `json:"Servicios"`
}

// DiscoverServiceIDs calls proxy.php?prxy=service/list using HMAC auth.
func DiscoverServiceIDs(sc *SecureClient) ([]servicioItem, error) {
	u := fmt.Sprintf("%s/proxy.php?prxy=service/list&lan=1&_=%d",
		SecurePanelBase, time.Now().UnixMilli())

	resp, err := sc.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, fmt.Errorf("service/list: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	var families []familiaItem
	if err := json.Unmarshal(body, &families); err != nil {
		return nil, fmt.Errorf("parse service/list: %w", err)
	}
	var out []servicioItem
	for _, f := range families {
		out = append(out, f.Servicios...)
	}
	return out, nil
}

type panellinkResponse struct {
	Type    string `json:"type"`
	Action  string `json:"action"`
	Encoded string `json:"encoded"`
}

// PanellinkToXSRF calls the panellink endpoint and follows the chain to
// get a cloudpanel XSRF token for a specific contracted service.
//
// Chain:
//
//	proxy.php?prxy=service/{idsco}/panellink
//	  → JSON {"action":"https://cloudpanel/.../loginuser.php?token=..."}
//	  → loginuser.php (302 + Set-Cookie: XSRF)
//	  → panel page (extract XSRF from cookie)
func PanellinkToXSRF(sc *SecureClient, idsco int) (string, time.Duration, error) {
	u := fmt.Sprintf("%s/proxy.php?prxy=service/%d/panellink&lan=1&_=%d",
		SecurePanelBase, idsco, time.Now().UnixMilli())

	resp, err := sc.Get(u)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return "", 0, fmt.Errorf("panellink: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return "", 0, err
	}
	var pl panellinkResponse
	if err := json.Unmarshal(body, &pl); err != nil {
		return "", 0, fmt.Errorf("parse panellink: %w", err)
	}
	if pl.Action == "" {
		return "", 0, fmt.Errorf("panellink: empty action")
	}

	loginURL := pl.Action
	if !strings.HasPrefix(loginURL, "https://") {
		loginURL = CloudPanelBase + "/" + strings.TrimLeft(loginURL, "/")
	}

	if Verbose {
		fmt.Printf("[verbose]   panellink action: %s\n", loginURL)
	}

	// loginuser.php redirect chain with cookie jar
	jar := makeCookieJar()
	redirectClient := &http.Client{
		Jar: jar,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if Verbose {
				fmt.Printf("[verbose]   loginuser redirect -> %s\n", r.URL.String())
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	loginResp, err := redirectClient.Get(loginURL)
	if err != nil {
		return "", 0, fmt.Errorf("loginuser: %w", err)
	}
	loginResp.Body.Close()

	if Verbose {
		fmt.Printf("[verbose]   loginuser final HTTP %d\n", loginResp.StatusCode)
	}

	// Extract XSRF from accumulated cookies
	allCookies := jar.AllCookies()
	if Verbose {
		for _, c := range allCookies {
			val := c.Value
			if len(val) > 60 {
				val = val[:60] + "..."
			}
			fmt.Printf("[verbose]   cookie: %s=%s (domain=%s)\n", c.Name, val, c.Domain)
		}
	}

	if tok, ttl := parseXSRFFromCookies(allCookies); tok != "" {
		if Verbose {
			fmt.Printf("[verbose]   XSRF token: %s (TTL: %v)\n", tok[:12]+"...", ttl)
		}
		return tok, ttl, nil
	}

	return "", 0, fmt.Errorf("could not extract XSRF token from cookies")
}

func parseXSRFFromCookies(cookies []*http.Cookie) (string, time.Duration) {
	for _, c := range cookies {
		if !strings.Contains(c.Name, "piensasolutions") {
			continue
		}
		decoded, err := url.QueryUnescape(c.Value)
		if err != nil {
			continue
		}
		if strings.HasPrefix(decoded, "j:") {
			decoded = decoded[2:]
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(decoded), &data); err != nil {
			continue
		}
		tok, _ := data["X-XSRF-TOKEN"].(string)
		if tok == "" {
			continue
		}
		var ttl time.Duration = 55 * time.Minute
		if timeout, ok := data["X-XSRF-TIMEOUT"].(float64); ok && timeout > 0 {
			ttl = time.Duration(timeout) * time.Second
		}
		return tok, ttl
	}
	return "", 0
}

// ValidateSecureToken checks whether the session is valid by calling service/list.
func ValidateSecureToken(sc *SecureClient) bool {
	_, err := DiscoverServiceIDs(sc)
	return err == nil
}
