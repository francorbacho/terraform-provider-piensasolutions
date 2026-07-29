package client

import (
	"encoding/json"
	"fmt"

	"github.com/fran/piensa/pkg/models"
)

type imagesResponse struct {
	Items []imageItem `json:"items"`
}

type imageItem struct {
	ID         string          `json:"id"`
	Properties imageProperties `json:"properties"`
}

type imageProperties struct {
	Name         string   `json:"name"`
	DatacenterID string   `json:"datacenter_id"`
	License      string   `json:"license"`
	Type         string   `json:"type"`
	Alias        string   `json:"alias"`
	ImageAliases []string `json:"image_aliases"`
	Source       string   `json:"source"`
}

// ListImages fetches all OS images (the HDD/cloud images that support
// cloud-init or bash init scripts on reinstall) visible to the account.
func ListImages(c *Client) ([]models.Image, error) {
	resp, err := c.get(FrontPanelBase + "/pss/images?depth=3")
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
	var ir imagesResponse
	if err := json.Unmarshal(body, &ir); err != nil {
		return nil, fmt.Errorf("parse images: %w", err)
	}
	var out []models.Image
	for _, item := range ir.Items {
		out = append(out, models.Image{
			ID:           item.ID,
			Name:         item.Properties.Name,
			DatacenterID: item.Properties.DatacenterID,
			License:      item.Properties.License,
			Type:         item.Properties.Type,
			Alias:        item.Properties.Alias,
			ImageAliases: item.Properties.ImageAliases,
			Source:       item.Properties.Source,
		})
	}
	return out, nil
}
