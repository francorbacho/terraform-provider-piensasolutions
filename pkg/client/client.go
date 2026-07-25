package client

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	CloudPanelBase  = "https://cloudpanel.piensasolutions.com"
	FrontPanelBase  = "https://front-cloudpanel.piensasolutions.com/api/corevps/v1"
	SecurePanelBase = "https://secure.piensasolutions.com"
	GatewayURL      = "https://secure.piensasolutions.com/public-gateway.php"
)

type Client struct {
	http   *http.Client
	token  string
	origin string
}

func New(token string) *Client {
	return &Client{
		http:   &http.Client{},
		token:  token,
		origin: CloudPanelBase,
	}
}

func (c *Client) WithOrigin(origin string) *Client {
	c.origin = origin
	return c
}

func (c *Client) do(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.origin)
	req.Header.Set("User-Agent", "piensa-cli/1.0")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	return resp, nil
}

func (c *Client) get(url string) (*http.Response, error) {
	return c.do(http.MethodGet, url, nil)
}

func (c *Client) post(url string, body interface{}) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return c.do(http.MethodPost, url, strings.NewReader(string(b)))
}

func (c *Client) put(url string, body interface{}) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return c.do(http.MethodPut, url, strings.NewReader(string(b)))
}

func (c *Client) del(url string) (*http.Response, error) {
	return c.do(http.MethodDelete, url, nil)
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := readBody(resp)
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

// --- HMAC client for secure.piensasolutions.com ---

type SecureClient struct {
	http   *http.Client
	token  string
	pvtKey string
}

func NewSecure(token, pvtKey string) *SecureClient {
	return &SecureClient{
		http:   &http.Client{},
		token:  token,
		pvtKey: pvtKey,
	}
}

func (sc *SecureClient) hmacHeaders() (string, string) {
	microtime := strconv.FormatFloat(float64(time.Now().UnixMicro())/1e6, 'f', 3, 64)
	mac := hmac.New(sha1.New, []byte(sc.pvtKey))
	mac.Write([]byte(sc.token + microtime))
	hash := hex.EncodeToString(mac.Sum(nil))
	return hash, microtime
}

func (sc *SecureClient) Get(u string) (*http.Response, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	hash, microtime := sc.hmacHeaders()
	req.Header.Set("X-TOKEN", sc.token)
	req.Header.Set("X-HASH", hash)
	req.Header.Set("X-MICROTIME", microtime)
	req.Header.Set("Origin", SecurePanelBase)
	req.Header.Set("Referer", SecurePanelBase+"/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "piensa-cli/1.0")
	resp, err := sc.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("secure request: %w", err)
	}
	return resp, nil
}

// SecureClientNoRedirect is like Get but doesn't follow redirects.
func (sc *SecureClient) GetNoRedirect(u string) (*http.Response, error) {
	noRedirect := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	hash, microtime := sc.hmacHeaders()
	req.Header.Set("X-TOKEN", sc.token)
	req.Header.Set("X-HASH", hash)
	req.Header.Set("X-MICROTIME", microtime)
	req.Header.Set("Origin", SecurePanelBase)
	req.Header.Set("Referer", SecurePanelBase+"/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "piensa-cli/1.0")
	resp, err := noRedirect.Do(req)
	if err != nil {
		return nil, fmt.Errorf("secure request: %w", err)
	}
	return resp, nil
}
