package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/francorbacho/terraform-provider-piensasolutions/pkg/client"
	"github.com/francorbacho/terraform-provider-piensasolutions/pkg/models"
)

// This pins down the two reverse-engineered fixes: "shutdown" must send the
// "suspend" action and "start" must send "resume" - the panel has no working
// /start or /shutdown server action. A naive revert back to the literal verb
// (e.g. makeActionCmd("shutdown", ..., "shutdown")) would 404 against the
// real API, so this test exists to catch that regression without needing
// live infrastructure.
func TestActionCommandMapping(t *testing.T) {
	want := map[string]string{
		"restart":  "reboot",
		"start":    "resume",
		"shutdown": "suspend",
		"suspend":  "suspend",
		"resume":   "resume",
	}

	got := make(map[string]string, len(actionCommandSpecs))
	for _, spec := range actionCommandSpecs {
		got[spec.Use] = spec.Action
	}

	if len(got) != len(want) {
		t.Fatalf("actionCommandSpecs has %d entries, want %d: %v", len(got), len(want), got)
	}
	for use, wantAction := range want {
		if gotAction, ok := got[use]; !ok {
			t.Errorf("missing action command %q", use)
		} else if gotAction != wantAction {
			t.Errorf("%q maps to action %q, want %q", use, gotAction, wantAction)
		}
	}
}

func sampleImages() []models.Image {
	return []models.Image{
		{
			ID:           "b97f8a37-8af5-11f1-affb-fadda67ef9cd",
			Name:         "IF-debian-13-generic-amd64.qcow2",
			DatacenterID: "es/vit",
			License:      "LINUX",
			Type:         "HDD",
			Alias:        "Debian 13",
			ImageAliases: []string{"IF-debian-13-generic-amd64"},
			Source:       "IMAGE_FACTORY",
		},
		{
			ID:           "88d2c879-8af2-11f1-affb-fadda67ef9cd",
			Name:         "IF-debian-12-generic-amd64.qcow2",
			DatacenterID: "es/vit",
			License:      "LINUX",
			Type:         "HDD",
			Alias:        "Debian 12",
			ImageAliases: []string{"IF-debian-12-generic-amd64"},
			Source:       "IMAGE_FACTORY",
		},
		{
			ID:           "1f3d38bd-8b01-11f1-affb-fadda67ef9cd",
			Name:         "IF-debian-13-generic-amd64_plesk18.qcow2",
			DatacenterID: "es/vit",
			License:      "LINUX",
			Type:         "HDD",
			Alias:        "Debian 13 + Plesk",
			ImageAliases: []string{"IF-debian-13-generic-amd64_plesk18"},
			Source:       "IMAGE_FACTORY",
		},
		{
			// Different datacenter - must never be picked for "es/vit".
			ID:           "a511c6a2-81e7-11f1-8767-b6f9a109277f",
			Name:         "windows-2022-server-2026-07",
			DatacenterID: "de/txl",
			License:      "WINDOWS2022",
			Type:         "HDD",
			Alias:        "windows 2022",
			ImageAliases: []string{"windows:2022"},
			Source:       "IONOS_CLOUD",
		},
		{
			// Same datacenter but a DVD/ISO, not an HDD image - reinstall
			// with cloud-init only supports HDD images, so this must never
			// be selected by findImage.
			ID:           "9b2f0b53-739c-11f1-bfd9-0acd6c77f08d",
			Name:         "Ubuntu-24.04.4-live-server-amd64.iso",
			DatacenterID: "es/vit",
			License:      "LINUX",
			Type:         "CDROM",
			Alias:        "ubuntu 24.04_iso",
			ImageAliases: []string{"ubuntu:24.04_iso"},
			Source:       "IONOS_CLOUD",
		},
	}
}

func TestFindImage(t *testing.T) {
	images := sampleImages()

	t.Run("exact alias match", func(t *testing.T) {
		img, err := findImage(images, "es/vit", "Debian 13")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if img.ID != "b97f8a37-8af5-11f1-affb-fadda67ef9cd" {
			t.Errorf("got image %q, want Debian 13", img.ID)
		}
	})

	t.Run("case-insensitive alias match", func(t *testing.T) {
		img, err := findImage(images, "es/vit", "debian 13")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if img.Alias != "Debian 13" {
			t.Errorf("got %q, want Debian 13", img.Alias)
		}
	})

	t.Run("exact image_aliases match", func(t *testing.T) {
		img, err := findImage(images, "es/vit", "IF-debian-12-generic-amd64")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if img.Alias != "Debian 12" {
			t.Errorf("got %q, want Debian 12", img.Alias)
		}
	})

	t.Run("exact ID match", func(t *testing.T) {
		img, err := findImage(images, "es/vit", "88d2c879-8af2-11f1-affb-fadda67ef9cd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if img.Alias != "Debian 12" {
			t.Errorf("got %q, want Debian 12", img.Alias)
		}
	})

	t.Run("unambiguous partial match", func(t *testing.T) {
		img, err := findImage(images, "es/vit", "plesk")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if img.Alias != "Debian 13 + Plesk" {
			t.Errorf("got %q, want Debian 13 + Plesk", img.Alias)
		}
	})

	t.Run("ambiguous partial match errors", func(t *testing.T) {
		_, err := findImage(images, "es/vit", "debian")
		if err == nil {
			t.Fatal("expected an error for an ambiguous match, got nil")
		}
	})

	t.Run("wrong datacenter is excluded", func(t *testing.T) {
		_, err := findImage(images, "es/vit", "windows 2022")
		if err == nil {
			t.Fatal("expected an error, windows 2022 image is in de/txl not es/vit")
		}
	})

	t.Run("non-HDD image is excluded", func(t *testing.T) {
		_, err := findImage(images, "es/vit", "ubuntu 24.04_iso")
		if err == nil {
			t.Fatal("expected an error, the ubuntu image is a CDROM/ISO not an HDD image")
		}
	})

	t.Run("no match errors", func(t *testing.T) {
		_, err := findImage(images, "es/vit", "does-not-exist")
		if err == nil {
			t.Fatal("expected an error for a nonexistent image")
		}
	})
}

func TestResolveLoginCredentials(t *testing.T) {
	env := map[string]string{
		"PIENSA_NIF":         "env-nif",
		"PIENSA_PASSWORD":    "env-password",
		"PIENSA_TOTP_SECRET": "JBSWY3DPEHPK3PXP",
	}
	getenv := func(k string) string { return env[k] }

	t.Run("all from env, code derived from totp secret", func(t *testing.T) {
		nif, password, code, err := resolveLoginCredentials("", "", "", "", getenv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nif != "env-nif" || password != "env-password" {
			t.Errorf("got nif=%q password=%q, want env values", nif, password)
		}
		if len(code) != 6 {
			t.Errorf("got code %q, want a 6-digit TOTP code", code)
		}
	})

	t.Run("explicit flags win over env", func(t *testing.T) {
		nif, password, code, err := resolveLoginCredentials("flag-nif", "flag-password", "123456", "", getenv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nif != "flag-nif" || password != "flag-password" || code != "123456" {
			t.Errorf("got nif=%q password=%q code=%q, want the explicit flag values unchanged", nif, password, code)
		}
	})

	t.Run("explicit 2fa code wins over totp secret", func(t *testing.T) {
		_, _, code, err := resolveLoginCredentials("", "", "654321", "JBSWY3DPEHPK3PXP", getenv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != "654321" {
			t.Errorf("got code %q, want the literal --2fa value 654321", code)
		}
	})

	t.Run("explicit totp-secret flag wins over env var", func(t *testing.T) {
		_, _, code, err := resolveLoginCredentials("", "", "", "JBSWY3DPEHPK3PXP", func(string) string { return "" })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != 6 {
			t.Errorf("got code %q, want a 6-digit TOTP code derived from the flag secret", code)
		}
	})

	t.Run("nothing set stays empty, no error", func(t *testing.T) {
		nif, password, code, err := resolveLoginCredentials("", "", "", "", func(string) string { return "" })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if nif != "" || password != "" || code != "" {
			t.Errorf("got nif=%q password=%q code=%q, want all empty so the interactive prompt kicks in", nif, password, code)
		}
	})

	t.Run("invalid totp secret errors instead of silently falling back", func(t *testing.T) {
		_, _, _, err := resolveLoginCredentials("n", "p", "", "not-valid-base32!!", func(string) string { return "" })
		if err == nil {
			t.Fatal("expected an error for an invalid TOTP secret, got nil")
		}
	})
}

func TestFindServer(t *testing.T) {
	const serverID = "11111111-1111-1111-1111-111111111111"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pss/servers" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"id": serverID,
					"properties": map[string]interface{}{
						"name":          "Test VPS",
						"state":         "ACTIVE",
						"power_state":   "ON",
						"os_name":       "Debian 13",
						"os_type":       "LINUX",
						"datacenter_id": "es/vit",
						"resources":     map[string]interface{}{"cpu": 2, "ram": 4.0, "disk": 120},
					},
					"related_entities": map[string]interface{}{"ip": []interface{}{}},
				},
			},
		})
	}))
	defer srv.Close()

	origBase := client.FrontPanelBase
	client.FrontPanelBase = srv.URL
	defer func() { client.FrontPanelBase = origBase }()

	c := client.New("test-token")

	found, err := findServer(c, serverID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.DatacenterID != "es/vit" {
		t.Errorf("got datacenter %q, want es/vit", found.DatacenterID)
	}

	if _, err := findServer(c, "does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown server ID")
	}
}
