package handlers

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests cover the TLS options shared by the two handlers that make
// outbound requests: the `http` handler and `wait` with `type: http`. Both must
// behave identically - verify against the system trust store by default, honor
// insecure_tls and ca_cert the same way, and reject the same misconfigurations.

// tlsEnv is the set of paths a case can point ca_cert at.
type tlsEnv struct {
	serverCert string // PEM of the test server's own (self-signed) certificate
	notPEM     string // a real file with no certificate in it
	missing    string // a path that does not exist
}

func newTLSEnv(t *testing.T, srv *httptest.Server) tlsEnv {
	t.Helper()

	dir := t.TempDir()

	certPath := filepath.Join(dir, "server.pem")
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if encoded == nil {
		t.Fatal("encoding the test server certificate as PEM produced nothing")
	}
	if err := os.WriteFile(certPath, encoded, 0o600); err != nil {
		t.Fatalf("writing %s: %v", certPath, err)
	}

	notPEM := filepath.Join(dir, "not-a-cert.pem")
	if err := os.WriteFile(notPEM, []byte("this file is not a certificate\n"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", notPEM, err)
	}

	return tlsEnv{
		serverCert: certPath,
		notPEM:     notPEM,
		missing:    filepath.Join(dir, "nope", "ca.pem"),
	}
}

const tlsProbeBody = "ok"

func TestHandlerTLSOptions(t *testing.T) {
	tests := []struct {
		name string
		// tls selects the self-signed httptest server over the plain one.
		tls bool
		// opts contributes the step options under test.
		opts func(tlsEnv) map[string]any
		// wantErr is a substring of the step error; empty means the step must
		// succeed.
		wantErr string
	}{
		{
			name: "https with no options rejects a self-signed certificate",
			tls:  true,
			// "certificate signed by unknown authority", wrapped differently
			// across Go versions.
			wantErr: "certificate",
		},
		{
			name:    "https with insecure_tls succeeds",
			tls:     true,
			opts:    func(tlsEnv) map[string]any { return map[string]any{"insecure_tls": true} },
			wantErr: "",
		},
		{
			name:    "https with insecure_tls as a string succeeds",
			tls:     true,
			opts:    func(tlsEnv) map[string]any { return map[string]any{"insecure_tls": "true"} },
			wantErr: "",
		},
		{
			name:    "https with insecure_tls false still verifies",
			tls:     true,
			opts:    func(tlsEnv) map[string]any { return map[string]any{"insecure_tls": false} },
			wantErr: "certificate",
		},
		{
			name:    "https trusting the server's own certificate succeeds",
			tls:     true,
			opts:    func(e tlsEnv) map[string]any { return map[string]any{"ca_cert": e.serverCert} },
			wantErr: "",
		},
		{
			name:    "ca_cert that does not exist is a step error",
			tls:     true,
			opts:    func(e tlsEnv) map[string]any { return map[string]any{"ca_cert": e.missing} },
			wantErr: "no such file or directory",
		},
		{
			name:    "ca_cert with no certificate in it is a step error",
			tls:     true,
			opts:    func(e tlsEnv) map[string]any { return map[string]any{"ca_cert": e.notPEM} },
			wantErr: "no PEM certificate found in file",
		},
		{
			name: "insecure_tls together with ca_cert is a step error",
			tls:  true,
			opts: func(e tlsEnv) map[string]any {
				return map[string]any{"insecure_tls": true, "ca_cert": e.serverCert}
			},
			wantErr: "mutually exclusive",
		},
		{
			name:    "unparseable insecure_tls is a step error",
			tls:     true,
			opts:    func(tlsEnv) map[string]any { return map[string]any{"insecure_tls": "yes please"} },
			wantErr: `invalid insecure_tls: "yes please" is not a boolean`,
		},
		{
			name:    "plain http with no options is unaffected",
			wantErr: "",
		},
		{
			name:    "plain http with insecure_tls is unaffected",
			opts:    func(tlsEnv) map[string]any { return map[string]any{"insecure_tls": true} },
			wantErr: "",
		},
		{
			name:    "plain http with ca_cert is unaffected",
			opts:    func(e tlsEnv) map[string]any { return map[string]any{"ca_cert": e.serverCert} },
			wantErr: "",
		},
		{
			name: "plain http still rejects insecure_tls with ca_cert",
			opts: func(e tlsEnv) map[string]any {
				return map[string]any{"insecure_tls": true, "ca_cert": e.serverCert}
			},
			wantErr: "mutually exclusive",
		},
	}

	handler := func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, tlsProbeBody)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both servers exist in every case: the TLS one is always the
			// source of the certificate a ca_cert case points at.
			tlsSrv := httptest.NewTLSServer(http.HandlerFunc(handler))
			defer tlsSrv.Close()
			plainSrv := httptest.NewServer(http.HandlerFunc(handler))
			defer plainSrv.Close()

			env := newTLSEnv(t, tlsSrv)

			url := plainSrv.URL
			if tt.tls {
				url = tlsSrv.URL
			}

			base := map[string]any{}
			if tt.opts != nil {
				base = tt.opts(env)
			}

			t.Run("http", func(t *testing.T) {
				step := map[string]any{"url": url, "timeout": "5s"}
				for k, v := range base {
					step[k] = v
				}

				result := (&HTTPHandler{}).Execute(step, testCtx())
				if tt.wantErr != "" {
					wantError(t, result, tt.wantErr)
					return
				}
				wantSuccess(t, result)
				if result.Stdout != tlsProbeBody {
					t.Errorf("stdout = %q, want %q", result.Stdout, tlsProbeBody)
				}
			})

			t.Run("wait", func(t *testing.T) {
				// Short deadline: the failing cases must not sit here for the
				// 30s default.
				step := map[string]any{
					"type":     "http",
					"url":      url,
					"timeout":  "600ms",
					"interval": "50ms",
				}
				for k, v := range base {
					step[k] = v
				}

				result := (&WaitHandler{}).Execute(step, testCtx())
				if tt.wantErr != "" {
					wantError(t, result, tt.wantErr)
					return
				}
				wantSuccess(t, result)
				if !strings.Contains(result.Stdout, "is ready") {
					t.Errorf("stdout = %q, want it to report readiness", result.Stdout)
				}
			})
		})
	}
}

// TestHandlerTLSCACertIsInterpolated proves ca_cert goes through the same
// interpolation as url, so ${env:...} paths work.
func TestHandlerTLSCACertIsInterpolated(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, tlsProbeBody)
	}))
	defer srv.Close()

	env := newTLSEnv(t, srv)
	t.Setenv("TSUITE_TEST_CA", env.serverCert)

	t.Run("http", func(t *testing.T) {
		result := (&HTTPHandler{}).Execute(map[string]any{
			"url":     srv.URL,
			"ca_cert": "${env:TSUITE_TEST_CA}",
		}, testCtx())
		wantSuccess(t, result)
	})

	t.Run("wait", func(t *testing.T) {
		result := (&WaitHandler{}).Execute(map[string]any{
			"type":     "http",
			"url":      srv.URL,
			"ca_cert":  "${env:TSUITE_TEST_CA}",
			"timeout":  "600ms",
			"interval": "50ms",
		}, testCtx())
		wantSuccess(t, result)
	})

	t.Run("uninterpolatable path still reports the resolved value", func(t *testing.T) {
		result := (&HTTPHandler{}).Execute(map[string]any{
			"url":     srv.URL,
			"ca_cert": "${env:TSUITE_TEST_CA}.missing",
		}, testCtx())
		wantError(t, result, env.serverCert+".missing")
	})
}

// TestHandlerTLSDefaultTransportUnchanged pins the promise that a step setting
// neither option gets exactly the client the handlers built before the options
// existed: no custom transport at all.
func TestHandlerTLSDefaultTransportUnchanged(t *testing.T) {
	client, err := newHTTPClient(defaultHTTPTimeout, tlsOptions{})
	if err != nil {
		t.Fatalf("newHTTPClient() error = %v", err)
	}
	if client.Transport != nil {
		t.Errorf("Transport = %#v, want nil so http.DefaultTransport is used", client.Transport)
	}
	if client.Timeout != defaultHTTPTimeout {
		t.Errorf("Timeout = %s, want %s", client.Timeout, defaultHTTPTimeout)
	}

	custom, err := newHTTPClient(defaultHTTPTimeout, tlsOptions{insecure: true})
	if err != nil {
		t.Fatalf("newHTTPClient(insecure) error = %v", err)
	}
	if custom.Transport == nil {
		t.Error("Transport = nil, want a cloned transport carrying the TLS config")
	}
}

// TestWaitHTTPTimeoutDiagnostics covers the two ways a wait can run out: a
// server that answers but never becomes ready, and a URL nothing is listening
// on. They used to produce the same message.
func TestWaitHTTPTimeoutDiagnostics(t *testing.T) {
	t.Run("persistent non-2xx names the status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		result := (&WaitHandler{}).Execute(map[string]any{
			"type":     "http",
			"url":      srv.URL,
			"timeout":  "300ms",
			"interval": "50ms",
		}, testCtx())

		wantError(t, result, "not ready after 300ms")
		wantError(t, result, "HTTP 503")
	})

	t.Run("unreachable port names the transport error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close() // nothing is listening now

		result := (&WaitHandler{}).Execute(map[string]any{
			"type":     "http",
			"url":      url,
			"timeout":  "300ms",
			"interval": "50ms",
		}, testCtx())

		wantError(t, result, "not ready after 300ms")
		wantError(t, result, "last attempt failed")
		wantError(t, result, "dial tcp")
		if strings.Contains(result.Error, "HTTP ") {
			t.Errorf("error = %q, want no status when no response was received", result.Error)
		}
	})

	t.Run("rejected certificate names the certificate", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer srv.Close()

		result := (&WaitHandler{}).Execute(map[string]any{
			"type":     "http",
			"url":      srv.URL,
			"timeout":  "300ms",
			"interval": "50ms",
		}, testCtx())

		wantError(t, result, "last attempt failed")
		wantError(t, result, "certificate")
	})
}
