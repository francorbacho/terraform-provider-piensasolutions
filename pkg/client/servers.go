package client

import (
	"encoding/json"
	"fmt"
)

// RawServerAction sends a PUT action and returns the raw response.
func RawServerAction(c *Client, serverID, action string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/pss/servers/%s/%s", FrontPanelBase, serverID, action)
	resp, err := c.put(url, struct{}{})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return result, nil
}
