package client

import (
	"encoding/json"
	"fmt"
)

type actionResponse struct {
	ID         string                      `json:"id"`
	Properties map[string]interface{}      `json:"properties"`
}

// RestartServer sends a reboot command.
func RestartServer(c *Client, serverID string) error {
	return putAction(c, serverID, "reboot")
}

// StartServer powers on the server.
func StartServer(c *Client, serverID string) error {
	return putAction(c, serverID, "start")
}

// ShutdownServer powers off the server.
func ShutdownServer(c *Client, serverID string) error {
	return putAction(c, serverID, "shutdown")
}

// SuspendServer suspends the server.
func SuspendServer(c *Client, serverID string) error {
	return putAction(c, serverID, "suspend")
}

// ResumeServer resumes a suspended server.
func ResumeServer(c *Client, serverID string) error {
	return putAction(c, serverID, "resume")
}

func putAction(c *Client, serverID, action string) error {
	url := fmt.Sprintf("%s/pss/servers/%s/%s", FrontPanelBase, serverID, action)
	resp, err := c.put(url, struct{}{})
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return fmt.Errorf("%s %s: %w", action, serverID, err)
	}
	return nil
}

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
