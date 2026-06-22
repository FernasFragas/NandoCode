package mcp

import (
	"context"
	"testing"
)

func TestValidateHTTPDestination(t *testing.T) {
	t.Parallel()
	mk := func(scheme, host, path string) string { return scheme + "://" + host + path }
	tests := []struct {
		name         string
		rawURL       string
		allowPrivate bool
		wantErr      bool
	}{
		{name: "https dns", rawURL: mk("https", "example.com", "/mcp"), wantErr: false},
		{name: "http localhost", rawURL: mk("http", "localhost:8080", "/mcp"), wantErr: false},
		{name: "http private denied", rawURL: mk("http", "10.0.0.1", "/mcp"), wantErr: true},
		{name: "https private denied", rawURL: mk("https", "10.0.0.1", "/mcp"), wantErr: true},
		{name: "https private allowed", rawURL: mk("https", "10.0.0.1", "/mcp"), allowPrivate: true, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHTTPDestination(tt.rawURL, tt.allowPrivate)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateHTTPDestination(%q) err=%v, wantErr=%v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDialHost(t *testing.T) {
	t.Parallel()
	if err := validateDialHost(context.Background(), "10.0.0.1", false); err == nil {
		t.Fatalf("expected private host to be rejected")
	}
	if err := validateDialHost(context.Background(), "10.0.0.1", true); err != nil {
		t.Fatalf("expected private host to be allowed when explicitly enabled: %v", err)
	}
}
