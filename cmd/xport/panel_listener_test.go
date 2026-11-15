package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPanelListenerOptsOutEvenWhenGODEBUGEnablesMPTCP(t *testing.T) {
	t.Setenv("GODEBUG", "multipathtcp=2")
	var defaultConfig net.ListenConfig
	if !defaultConfig.MultipathTCP() {
		t.Fatal("test setup: Go MPTCP default was not enabled by GODEBUG")
	}
	lc := panelListenConfig()
	if lc.MultipathTCP() {
		t.Fatal("panel listener must opt out of MPTCP, even with GODEBUG=multipathtcp=2")
	}
	ln, err := panelListener("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if _, ok := ln.(*net.TCPListener); !ok {
		t.Fatalf("expected TCP listener, got %T", ln)
	}
}

func TestPanelListenerServesHTTP(t *testing.T) {
	ln, err := panelListener("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "panel-http-ok")
	})}
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(ln) }()
	defer shutdownPanelTestServer(t, srv, serveDone)

	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "panel-http-ok" {
		t.Fatalf("HTTP response: status=%d body=%q", resp.StatusCode, body)
	}
}

func TestPanelListenerServesHTTPS(t *testing.T) {
	// The test server provides a valid local test certificate without
	// introducing certificates or secrets into repository files.
	donor := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer donor.Close()
	identity := donor.TLS.Certificates[0]
	keyDER, err := x509.MarshalPKCS8PrivateKey(identity.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: identity.Certificate[0]})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}

	ln, err := panelListener("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "panel-https-ok")
	})}
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.ServeTLS(ln, certFile, keyFile) }()
	defer shutdownPanelTestServer(t, srv, serveDone)

	client := donor.Client()
	client.Timeout = 5 * time.Second
	resp, err := client.Get("https://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "panel-https-ok" {
		t.Fatalf("HTTPS response: status=%d body=%q", resp.StatusCode, body)
	}
}

func shutdownPanelTestServer(t *testing.T, srv *http.Server, done <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("shutdown panel test server: %v", err)
	}
	if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) &&
		!strings.Contains(err.Error(), "use of closed network connection") {
		t.Errorf("serve panel test server: %v", err)
	}
}
