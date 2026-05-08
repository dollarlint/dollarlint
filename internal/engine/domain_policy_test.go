package engine

import (
	"strings"
	"testing"
)

func TestDomainPolicyEdges(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Schemas.Fetch.AllowedDomains = []string{"https://schemas.example.com:443/path", "*.example.org:443", ""}
	if err := checkRemoteDomainPolicy("https://schemas.example.com/schema.json", cfg.Schemas); err != nil {
		t.Fatalf("expected exact URL-pattern domain allow: %v", err)
	}
	if err := checkRemoteDomainPolicy("https://api.example.org/schema.json", cfg.Schemas); err != nil {
		t.Fatalf("expected wildcard domain allow: %v", err)
	}
	if err := checkRemoteDomainPolicy("https://example.org/schema.json", cfg.Schemas); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected wildcard not to match root domain, got %v", err)
	}
	cfg.Schemas.Fetch.BlockedDomains = []string{"//api.example.org"}
	if err := checkRemoteDomainPolicy("https://api.example.org/schema.json", cfg.Schemas); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked URL-pattern domain, got %v", err)
	}
	cfg.Schemas.Fetch.BlockedDomains = []string{"bad domain"}
	if err := checkRemoteDomainPolicy("https://schemas.example.com/schema.json", cfg.Schemas); err == nil || !strings.Contains(err.Error(), "invalid schema domain") {
		t.Fatalf("expected invalid blocked domain, got %v", err)
	}
	cfg.Schemas.Fetch.BlockedDomains = nil
	cfg.Schemas.Fetch.AllowedDomains = []string{"bad/domain"}
	if err := checkRemoteDomainPolicy("https://schemas.example.com/schema.json", cfg.Schemas); err == nil || !strings.Contains(err.Error(), "invalid schema domain") {
		t.Fatalf("expected invalid allowed domain, got %v", err)
	}
	if err := checkRemoteDomainPolicy("file:///tmp/schema.json", cfg.Schemas); err == nil || !strings.Contains(err.Error(), "has no host") {
		t.Fatalf("expected no-host policy error, got %v", err)
	}
	if _, err := hostForDomainPolicy("file:///tmp/schema.json"); err == nil || !strings.Contains(err.Error(), "has no host") {
		t.Fatalf("expected no-host error, got %v", err)
	}
	if _, err := hostForDomainPolicy("http://[::1"); err == nil || !strings.Contains(err.Error(), "parse schema URL") {
		t.Fatalf("expected parse error, got %v", err)
	}
	if _, err := normalizeDomainPattern("//%"); err == nil || !strings.Contains(err.Error(), "invalid schema domain") {
		t.Fatalf("expected protocol-relative parse error, got %v", err)
	}
	if _, err := normalizeDomainPattern("//"); err == nil || !strings.Contains(err.Error(), "invalid schema domain") {
		t.Fatalf("expected empty protocol-relative domain error, got %v", err)
	}
	if _, err := normalizeDomainPattern("*.bad domain"); err == nil || !strings.Contains(err.Error(), "invalid schema domain") {
		t.Fatalf("expected wildcard invalid domain, got %v", err)
	}
	if value, err := normalizeDomainPattern("example.com:443"); err != nil || value != "example.com" {
		t.Fatalf("host:port normalization = %q, %v", value, err)
	}
}
