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

type entrypointResponse struct {
	Token string `json:"token"`
}

type serverItem struct {
	ID         string            `json:"id"`
	Properties serverProperties  `json:"properties"`
	Related    relatedEntities   `json:"related_entities"`
}

type serverProperties struct {
	Name         string           `json:"name"`
	State        string           `json:"state"`
	PowerState   string           `json:"power_state"`
	OSName       string           `json:"os_name"`
	OSType       string           `json:"os_type"`
	DatacenterID string           `json:"datacenter_id"`
	Resources    models.ServerResources `json:"resources"`
}

type relatedEntities struct {
	IPs []ipItem `json:"ip"`
}

type ipItem struct {
	ID         string      `json:"id"`
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

// DiscoverServers fetches all servers visible to a given X-TOKEN.
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
			Raw:          item,
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

// ValidateToken checks whether the X-TOKEN is still valid by calling entrypoint.
// Returns the token from the response (should match) and the remaining TTL hint.
func ValidateToken(c *Client) (bool, error) {
	resp, err := c.get(FrontPanelBase + "/entrypoint")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

// RefreshTokenViaPanellink obtains a fresh X-TOKEN for a specific contracted service.
// It calls the secure panel's panellink endpoint with the secure token,
// follows the redirect to loginuser.php, and extracts the new XSRF token.
//
// Steps:
//  1. GET secure.piensasolutions.com/proxy.php?prxy=service/{idsco}/panellink
//  2. Follow 302 to cloudpanel.piensasolutions.com/loginuser.php?token=...
//  3. Parse Set-Cookie for X-XSRF-TOKEN
func RefreshTokenViaPanellink(secureToken string, idsco string) (string, time.Duration, error) {
	linkClient := New(secureToken).WithOrigin(SecurePanelBase)

	// Step 1: get panellink URL
	panellinkURL := fmt.Sprintf("%s/proxy.php?prxy=service/%s/panellink&lan=1",
		SecurePanelBase, idsco)
	resp, err := linkClient.get(panellinkURL)
	if err != nil {
		return "", 0, fmt.Errorf("panellink: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return "", 0, err
	}
	if err := checkStatus(resp); err != nil {
		return "", 0, fmt.Errorf("panellink %s: %w", idsco, err)
	}

	var plResp struct {
		Type   string `json:"type"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(body, &plResp); err != nil {
		return "", 0, fmt.Errorf("parse panellink: %w", err)
	}
	if plResp.Action == "" {
		return "", 0, fmt.Errorf("empty panellink action")
	}

	// Step 2: follow redirect
	loginURL := plResp.Action
	if !strings.HasPrefix(loginURL, "https://") {
		loginURL = CloudPanelBase + "/" + strings.TrimLeft(loginURL, "/")
	}

	anonClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, _ := http.NewRequest("GET", loginURL, nil)
	req.Header.Set("User-Agent", "piensa-cli/1.0")
	stepResp, err := anonClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("loginuser: %w", err)
	}
	defer stepResp.Body.Close()

	if stepResp.StatusCode != 302 && stepResp.StatusCode != 200 {
		return "", 0, fmt.Errorf("loginuser unexpected status: %d", stepResp.StatusCode)
	}

	// Step 3: follow the redirect to panel to get the session cookie with XSRF
	var location string
	if stepResp.StatusCode == 302 {
		location = stepResp.Header.Get("Location")
		if location == "" {
			return "", 0, fmt.Errorf("no location header in redirect")
		}
		if !strings.HasPrefix(location, "https://") {
			location = CloudPanelBase + "/" + strings.TrimLeft(location, "/")
		}
	}

	// Follow to the panel page
	finalResp, err := anonClient.Get(location)
	if err != nil {
		return "", 0, fmt.Errorf("panel redirect: %w", err)
	}
	defer finalResp.Body.Close()

	// Extract XSRF token from cookies
	for _, c := range finalResp.Cookies() {
		if strings.Contains(c.Name, "piensasolutions") {
			// Cookie value is URL-encoded JSON: j%3A%7B%22X-XSRF-TOKEN%22%3A...
			decoded, err := url.QueryUnescape(c.Value)
			if err != nil {
				continue
			}
			if strings.HasPrefix(decoded, "j:") {
				decoded = decoded[2:]
			}
			var cookieData map[string]interface{}
			if err := json.Unmarshal([]byte(decoded), &cookieData); err != nil {
				continue
			}
			if token, ok := cookieData["X-XSRF-TOKEN"].(string); ok && token != "" {
				// XSRF timeout is in the cookie as well
				var ttl time.Duration
				if timeout, ok := cookieData["X-XSRF-TIMEOUT"].(float64); ok {
					ttl = time.Duration(timeout) * time.Second
				} else {
					ttl = 55 * time.Minute // default safe estimate
				}
				return token, ttl, nil
			}
		}
	}
	return "", 0, fmt.Errorf("could not extract XSRF token from panel cookies")
}

// DiscoverAllServers tries each token in the account and returns all discovered servers.
// It also returns a map of server ID → token index for later use.
func DiscoverAllServers(tokens []string) ([]models.Server, map[string]string, error) {
	var all []models.Server
	tokenMap := make(map[string]string)
	seen := make(map[string]bool)

	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		c := New(tok)
		servers, err := DiscoverServers(c)
		if err != nil {
			continue
		}
		for _, s := range servers {
			if !seen[s.ID] {
				seen[s.ID] = true
				all = append(all, s)
				tokenMap[s.ID] = tok
			}
		}
	}
	return all, tokenMap, nil
}
