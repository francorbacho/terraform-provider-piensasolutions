package client

import (
	"encoding/json"
	"fmt"
)

// ReinstallRequest describes an OS reinstall ("reflash") of a server from an
// IMAGE-type template (the cloud images returned by ListImages), optionally
// seeded with a cloud-init config or a bash init script run on first boot.
type ReinstallRequest struct {
	ImageID string
	// Password is the new root/administrator password. Empty means the
	// server auto-generates one, returned as "first_password" in the response.
	Password string
	// CloudConfigContentType is "yaml" (cloud-init, #cloud-config) or "sh"
	// (bash script, #!/bin/bash). Empty means no init config is sent.
	CloudConfigContentType string
	CloudConfig            string
}

// ReinstallServer wipes and reinstalls a server's OS. This destroys all data
// on the server's disk.
func ReinstallServer(c *Client, serverID string, req ReinstallRequest) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"image_type": "IMAGE",
		"image_id":   req.ImageID,
	}
	if req.Password != "" {
		body["password"] = req.Password
	} else {
		body["password"] = nil
	}
	if req.CloudConfigContentType != "" {
		body["cloud_config_content_type"] = req.CloudConfigContentType
		body["cloud_config"] = req.CloudConfig
	} else {
		body["cloud_config_content_type"] = nil
		body["cloud_config"] = nil
	}

	url := fmt.Sprintf("%s/pss/servers/%s/reinstall", FrontPanelBase, serverID)
	resp, err := c.put(url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	respBody, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return result, nil
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
