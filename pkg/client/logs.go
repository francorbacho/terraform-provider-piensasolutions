package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/francorbacho/terraform-provider-piensasolutions/pkg/models"
)

type logsResponse struct {
	Items []logItem `json:"items"`
}

type logItem struct {
	ID         string        `json:"id"`
	Properties logProperties `json:"properties"`
}

type logProperties struct {
	Action   string          `json:"action"`
	Status   string          `json:"status"`
	User     string          `json:"user"`
	Started  string          `json:"started"`
	Finished *string         `json:"finished"`
	Details  json.RawMessage `json:"details"`
}

// ListLogs fetches the action history (start/stop/reinstall/... events) for
// a single server, most recent first. This hits a separate backend service
// (pss-core) from the rest of the API and appears to enforce token freshness
// more strictly, so a 401 here can mean "log in again" even when other
// commands still work with the same token.
func ListLogs(c *Client, serverID string, limit int) ([]models.LogEntry, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("sort", "-started")
	q.Set("filter[element.id]", serverID)

	resp, err := c.get(FrontPanelBase + "/pss-core/logs?" + q.Encode())
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
	var lr logsResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return nil, fmt.Errorf("parse logs: %w", err)
	}

	var out []models.LogEntry
	for _, item := range lr.Items {
		entry := models.LogEntry{
			ID:     item.ID,
			Action: item.Properties.Action,
			Status: item.Properties.Status,
			User:   item.Properties.User,
		}
		if t, err := time.Parse(time.RFC3339, item.Properties.Started); err == nil {
			entry.Started = t
		}
		if item.Properties.Finished != nil {
			if t, err := time.Parse(time.RFC3339, *item.Properties.Finished); err == nil {
				entry.Finished = &t
			}
		}
		out = append(out, entry)
	}
	return out, nil
}
