package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newUpsertServer serves an empty suite list on GET /api/suites and the given
// status/body on POST /api/suites.
func newUpsertServer(t *testing.T, createStatus int, createBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/suites" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"suites":[],"count":0}`))
			return
		}
		w.WriteHeader(createStatus)
		_, _ = w.Write([]byte(createBody))
	}))
}

func TestUpsertSuiteCreated(t *testing.T) {
	srv := newUpsertServer(t, http.StatusCreated, `{"id":42,"folder_path":"/suites/demo","suite_name":"demo"}`)
	defer srv.Close()

	resp, err := NewClient(srv.URL).UpsertSuite(&SyncSuiteRequest{FolderPath: "/suites/demo", SuiteName: "demo"})
	if err != nil {
		t.Fatalf("UpsertSuite() error = %v", err)
	}
	if resp.ID != 42 {
		t.Errorf("ID = %d, want 42", resp.ID)
	}
	if resp.SuiteName != "demo" {
		t.Errorf("SuiteName = %q, want %q", resp.SuiteName, "demo")
	}
}

func TestUpsertSuiteCreatedWithoutID(t *testing.T) {
	srv := newUpsertServer(t, http.StatusCreated, `{"folder_path":"/suites/demo","suite_name":"demo"}`)
	defer srv.Close()

	if _, err := NewClient(srv.URL).UpsertSuite(&SyncSuiteRequest{FolderPath: "/suites/demo"}); err == nil {
		t.Fatal("UpsertSuite() error = nil, want error for missing suite id")
	}
}

func TestUpsertSuiteConflict(t *testing.T) {
	srv := newUpsertServer(t, http.StatusConflict,
		`{"error":"Suite already exists","suite":{"id":7,"folder_path":"/suites/demo","suite_name":"demo"}}`)
	defer srv.Close()

	resp, err := NewClient(srv.URL).UpsertSuite(&SyncSuiteRequest{FolderPath: "/suites/demo"})
	if err != nil {
		t.Fatalf("UpsertSuite() error = %v", err)
	}
	if resp.ID != 7 {
		t.Errorf("ID = %d, want 7", resp.ID)
	}
}

func TestUpsertSuiteServerError(t *testing.T) {
	srv := newUpsertServer(t, http.StatusInternalServerError, `{"error":"database is locked"}`)
	defer srv.Close()

	_, err := NewClient(srv.URL).UpsertSuite(&SyncSuiteRequest{FolderPath: "/suites/demo"})
	if err == nil {
		t.Fatal("UpsertSuite() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want it to mention the HTTP status", err)
	}
	if !strings.Contains(err.Error(), "database is locked") {
		t.Errorf("error = %q, want it to mention the server error field", err)
	}
}

func TestUpsertSuiteExisting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s request to %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"suites":[{"id":3,"folder_path":"/suites/demo","suite_name":"demo"}],"count":1}`))
	}))
	defer srv.Close()

	resp, err := NewClient(srv.URL).UpsertSuite(&SyncSuiteRequest{FolderPath: "/suites/demo"})
	if err != nil {
		t.Fatalf("UpsertSuite() error = %v", err)
	}
	if resp.ID != 3 {
		t.Errorf("ID = %d, want 3", resp.ID)
	}
}
