package client_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/francorbacho/terraform-provider-piensasolutions/pkg/client"
)

// sampleLogsJSON mirrors the shapes actually observed in the HAR captures:
// "details" comes back as either an object or an empty array, and
// "finished" is null for an in-progress action.
const sampleLogsJSON = `{"items":[
	{"id":"31622300-9d24-4e79-afd3-ae15f041ecb3","properties":{"element":{"id":"11111111-1111-1111-1111-111111111111","name":"Test VPS","type":"SERVER"},"action":"reinstall","started":"2026-01-02T03:04:05Z","finished":"2026-01-02T03:05:06Z","status":"SUCCESS","user":"test","details":{"DATACENTER_LOCATION":"es/vit","SERVER_NAME":"Test VPS"}}},
	{"id":"835596f8-2cf8-4cf8-a2a4-384b5821059a","properties":{"element":{"id":"11111111-1111-1111-1111-111111111111","name":"Test VPS","type":"SERVER"},"action":"suspend","started":"2026-01-01T22:11:12Z","finished":null,"status":"IN_PROGRESS","user":"test","details":{"DATACENTER_LOCATION":"es/vit","SERVER_NAME":"Test VPS"}}},
	{"id":"b36dd3d0-5935-4c31-8255-c60b1e2d2c5f","properties":{"element":{"id":"11111111-1111-1111-1111-111111111111","name":"","type":"SERVER"},"action":"reboot","started":"2026-01-03T09:08:07Z","finished":"2026-01-03T09:08:07Z","status":"FAILED","user":"test","details":[]}}
],"prev_offset":0,"next_offset":5}`

func TestListLogs_QueryParams(t *testing.T) {
	var gotPath, gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[]}`)
	}))
	defer srv.Close()

	origBase := client.FrontPanelBase
	client.FrontPanelBase = srv.URL
	defer func() { client.FrontPanelBase = origBase }()

	c := client.New("test-token")
	if _, err := client.ListLogs(c, "11111111-1111-1111-1111-111111111111", 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/pss-core/logs" {
		t.Errorf("got path %q, want /pss-core/logs", gotPath)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("failed to parse query %q: %v", gotQuery, err)
	}
	if q.Get("limit") != "5" {
		t.Errorf("got limit=%q, want 5", q.Get("limit"))
	}
	if q.Get("sort") != "-started" {
		t.Errorf("got sort=%q, want -started", q.Get("sort"))
	}
	if q.Get("filter[element.id]") != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("got filter[element.id]=%q, want the server ID", q.Get("filter[element.id]"))
	}
}

func TestListLogs_ParsesEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, sampleLogsJSON)
	}))
	defer srv.Close()

	origBase := client.FrontPanelBase
	client.FrontPanelBase = srv.URL
	defer func() { client.FrontPanelBase = origBase }()

	c := client.New("test-token")
	entries, err := client.ListLogs(c, "11111111-1111-1111-1111-111111111111", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	reinstall := entries[0]
	if reinstall.Action != "reinstall" || reinstall.Status != "SUCCESS" || reinstall.User != "test" {
		t.Errorf("unexpected reinstall entry: %+v", reinstall)
	}
	wantStarted := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if !reinstall.Started.Equal(wantStarted) {
		t.Errorf("got started %v, want %v", reinstall.Started, wantStarted)
	}
	if reinstall.Finished == nil {
		t.Fatal("expected a non-nil Finished time for the completed reinstall")
	}

	inProgress := entries[1]
	if inProgress.Status != "IN_PROGRESS" {
		t.Errorf("got status %q, want IN_PROGRESS", inProgress.Status)
	}
	if inProgress.Finished != nil {
		t.Errorf("expected a nil Finished time for an in-progress action, got %v", inProgress.Finished)
	}

	// "details":[] (an array, not an object) must not break parsing of the
	// rest of the entry - this shape was observed for FAILED actions.
	failed := entries[2]
	if failed.Action != "reboot" || failed.Status != "FAILED" {
		t.Errorf("unexpected failed entry: %+v", failed)
	}
}
