package client_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/francorbacho/terraform-provider-piensasolutions/pkg/client"
)

const sampleReinstallResponse = `{"id":"11111111-1111-1111-1111-111111111111","properties":{"name":"Test VPS","state":"CONFIGURING","image_id":"b97f8a37-8af5-11f1-affb-fadda67ef9cd","power_state":"ON","first_password":"test-auto-password","resources":{"cpu":2,"ram":4,"disk":120}}}`

func TestReinstallServer_RequestBody(t *testing.T) {
	tests := []struct {
		name string
		req  client.ReinstallRequest
		want map[string]interface{}
	}{
		{
			name: "no password, no cloud config",
			req: client.ReinstallRequest{
				ImageID: "b97f8a37-8af5-11f1-affb-fadda67ef9cd",
			},
			want: map[string]interface{}{
				"password":                  nil,
				"image_type":                "IMAGE",
				"image_id":                  "b97f8a37-8af5-11f1-affb-fadda67ef9cd",
				"cloud_config_content_type": nil,
				"cloud_config":              nil,
			},
		},
		{
			name: "cloud-init yaml",
			req: client.ReinstallRequest{
				ImageID:                "b97f8a37-8af5-11f1-affb-fadda67ef9cd",
				CloudConfigContentType: "yaml",
				CloudConfig:            "#cloud-config\nruncmd: [echo hi]\n",
			},
			want: map[string]interface{}{
				"password":                  nil,
				"image_type":                "IMAGE",
				"image_id":                  "b97f8a37-8af5-11f1-affb-fadda67ef9cd",
				"cloud_config_content_type": "yaml",
				"cloud_config":              "#cloud-config\nruncmd: [echo hi]\n",
			},
		},
		{
			name: "bash script and custom password",
			req: client.ReinstallRequest{
				ImageID:                "b97f8a37-8af5-11f1-affb-fadda67ef9cd",
				Password:               "custom-pw",
				CloudConfigContentType: "sh",
				CloudConfig:            "#!/bin/bash\necho hi\n",
			},
			want: map[string]interface{}{
				"password":                  "custom-pw",
				"image_type":                "IMAGE",
				"image_id":                  "b97f8a37-8af5-11f1-affb-fadda67ef9cd",
				"cloud_config_content_type": "sh",
				"cloud_config":              "#!/bin/bash\necho hi\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			var gotBody map[string]interface{}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &gotBody); err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, sampleReinstallResponse)
			}))
			defer srv.Close()

			origBase := client.FrontPanelBase
			client.FrontPanelBase = srv.URL
			defer func() { client.FrontPanelBase = origBase }()

			c := client.New("test-token")
			result, err := client.ReinstallServer(c, "11111111-1111-1111-1111-111111111111", tt.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gotMethod != http.MethodPut {
				t.Errorf("got method %s, want PUT", gotMethod)
			}
			if gotPath != "/pss/servers/11111111-1111-1111-1111-111111111111/reinstall" {
				t.Errorf("got path %q", gotPath)
			}
			for k, wantV := range tt.want {
				if gotV := gotBody[k]; gotV != wantV {
					t.Errorf("body[%q] = %v, want %v", k, gotV, wantV)
				}
			}

			props, ok := result["properties"].(map[string]interface{})
			if !ok {
				t.Fatalf("response missing properties: %v", result)
			}
			if props["first_password"] != "test-auto-password" {
				t.Errorf("got first_password %v, want test-auto-password", props["first_password"])
			}
		})
	}
}

func TestReinstallServer_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, "conflict")
	}))
	defer srv.Close()

	origBase := client.FrontPanelBase
	client.FrontPanelBase = srv.URL
	defer func() { client.FrontPanelBase = origBase }()

	c := client.New("test-token")
	_, err := client.ReinstallServer(c, "11111111-1111-1111-1111-111111111111", client.ReinstallRequest{ImageID: "x"})
	if err == nil {
		t.Fatal("expected an error for HTTP 409, got nil")
	}
	if err.Error() != "server is busy, try again in a few seconds" {
		t.Errorf("got error %q, want the 409 busy message", err.Error())
	}
}

func TestRawServerAction(t *testing.T) {
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"11111111-1111-1111-1111-111111111111","properties":{"power_state":"SUSPENDING"}}`)
	}))
	defer srv.Close()

	origBase := client.FrontPanelBase
	client.FrontPanelBase = srv.URL
	defer func() { client.FrontPanelBase = origBase }()

	c := client.New("test-token")
	result, err := client.RawServerAction(c, "11111111-1111-1111-1111-111111111111", "suspend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("got method %s, want PUT", gotMethod)
	}
	if gotPath != "/pss/servers/11111111-1111-1111-1111-111111111111/suspend" {
		t.Errorf("got path %q, want the /suspend action path", gotPath)
	}
	props, _ := result["properties"].(map[string]interface{})
	if props["power_state"] != "SUSPENDING" {
		t.Errorf("got power_state %v, want SUSPENDING", props["power_state"])
	}
}
