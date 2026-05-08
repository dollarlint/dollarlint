package engine

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func checkRemoteDomainPolicy(raw string, cfg SchemaConfig) error {
	if !remoteFetchEnabled(cfg) {
		return fmt.Errorf("remote schema fetching disabled for %s", raw)
	}
	host, err := hostForDomainPolicy(raw)
	if err != nil {
		return err
	}
	if matched, err := matchesAnyDomainPattern(host, cfg.BlockedDomains); err != nil {
		return err
	} else if matched {
		return fmt.Errorf("remote schema domain %q is blocked by configuration", host)
	}
	if countNonEmptyDomains(cfg.AllowedDomains) == 0 {
		return nil
	}
	if matched, err := matchesAnyDomainPattern(host, cfg.AllowedDomains); err != nil {
		return err
	} else if !matched {
		return fmt.Errorf("remote schema domain %q is not allowed by configuration", host)
	}
	return nil
}

func matchesAnyDomainPattern(host string, patterns []string) (bool, error) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, raw := range patterns {
		pattern, err := normalizeDomainPattern(raw)
		if err != nil {
			return false, err
		}
		if pattern == "" {
			continue
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*.")
			if host != suffix && strings.HasSuffix(host, "."+suffix) {
				return true, nil
			}
			continue
		}
		if host == pattern {
			return true, nil
		}
	}
	return false, nil
}

func normalizeDomainPattern(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if value == "" {
		return "", nil
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		value = parsed.Hostname()
	} else if strings.HasPrefix(value, "//") {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", fmt.Errorf("invalid schema domain %q: %w", raw, err)
		}
		value = parsed.Hostname()
	}
	if strings.HasPrefix(value, "*.") {
		suffix := strings.TrimPrefix(value, "*.")
		if suffix == "" || strings.ContainsAny(suffix, "/\\") {
			return "", fmt.Errorf("invalid schema domain %q", raw)
		}
		return "*." + suffix, nil
	}
	value = strings.Trim(value, "[]")
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = strings.Trim(host, "[]")
	}
	if value == "" || strings.ContainsAny(value, "/\\") {
		return "", fmt.Errorf("invalid schema domain %q", raw)
	}
	return value, nil
}

func hostForDomainPolicy(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse schema URL %s: %w", raw, err)
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return "", fmt.Errorf("schema URL %s has no host", raw)
	}
	return host, nil
}

func countNonEmptyDomains(domains []string) int {
	count := 0
	for _, domain := range domains {
		if strings.TrimSpace(domain) != "" {
			count++
		}
	}
	return count
}
