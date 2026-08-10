package client_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/francorbacho/terraform-provider-piensasolutions/pkg/client"
)

// sampleImagesJSON is a trimmed version of a real /pss/images?depth=3
// response captured in reflashing.har.
const sampleImagesJSON = `{"items":[
	{"id":"b97f8a37-8af5-11f1-affb-fadda67ef9cd","properties":{"name":"IF-debian-13-generic-amd64.qcow2","datacenter_id":"es/vit","license":"LINUX","type":"HDD","alias":"Debian 13","image_aliases":["IF-debian-13-generic-amd64"],"source":"IMAGE_FACTORY"}},
	{"id":"5e5ae9fd-81eb-11f1-8767-b6f9a109277f","properties":{"name":"windows-2022-server-2026-07","datacenter_id":"es/vit","license":"WINDOWS2022","type":"HDD","alias":"windows 2022","image_aliases":["windows:2022"],"source":"IONOS_CLOUD"}}
]}`

func TestListImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("got method %s, want GET", r.Method)
		}
		if r.URL.Path != "/pss/images" {
			t.Errorf("got path %q, want /pss/images", r.URL.Path)
		}
		if got := r.URL.Query().Get("depth"); got != "3" {
			t.Errorf("got depth=%q, want 3", got)
		}
		if got := r.Header.Get("X-TOKEN"); got != "test-token" {
			t.Errorf("got X-TOKEN=%q, want test-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, sampleImagesJSON)
	}))
	defer srv.Close()

	origBase := client.FrontPanelBase
	client.FrontPanelBase = srv.URL
	defer func() { client.FrontPanelBase = origBase }()

	c := client.New("test-token")
	images, err := client.ListImages(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("got %d images, want 2", len(images))
	}

	debian := images[0]
	if debian.Alias != "Debian 13" || debian.DatacenterID != "es/vit" || debian.Type != "HDD" || debian.License != "LINUX" {
		t.Errorf("unexpected first image: %+v", debian)
	}
	if len(debian.ImageAliases) != 1 || debian.ImageAliases[0] != "IF-debian-13-generic-amd64" {
		t.Errorf("unexpected image aliases: %+v", debian.ImageAliases)
	}

	windows := images[1]
	if windows.Alias != "windows 2022" || windows.License != "WINDOWS2022" {
		t.Errorf("unexpected second image: %+v", windows)
	}
}

func TestListImages_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "forbidden")
	}))
	defer srv.Close()

	origBase := client.FrontPanelBase
	client.FrontPanelBase = srv.URL
	defer func() { client.FrontPanelBase = origBase }()

	c := client.New("test-token")
	if _, err := client.ListImages(c); err == nil {
		t.Fatal("expected an error for HTTP 403, got nil")
	}
}
