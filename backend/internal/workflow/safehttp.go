package workflow

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ValidatePublicURL checks if rawURL is valid and does not resolve to private or loopback hosts.
func ValidatePublicURL(ctx context.Context, rawURL string, allowPrivate bool) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid scheme %q: only http and https are allowed", u.Scheme)
	}

	if !allowPrivate {
		if err := checkHost(ctx, u.Host); err != nil {
			return nil, fmt.Errorf("host blocked (%s): %w", u.Host, err)
		}
	}
	return u, nil
}

// ValidateSlackWebhookURL ensures the webhook URL is a genuine Slack endpoint.
func ValidateSlackWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid slack webhook url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("slack webhook must use https")
	}
	host := strings.ToLower(u.Hostname())
	if host != "hooks.slack.com" && !strings.HasSuffix(host, ".slack.com") {
		return fmt.Errorf("invalid slack host %q: must be hooks.slack.com", host)
	}
	return nil
}

// ValidateDiscordWebhookURL ensures the webhook URL is a genuine Discord endpoint.
func ValidateDiscordWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid discord webhook url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("discord webhook must use https")
	}
	host := strings.ToLower(u.Hostname())
	if host != "discord.com" && host != "discordapp.com" && !strings.HasSuffix(host, ".discord.com") && !strings.HasSuffix(host, ".discordapp.com") {
		return fmt.Errorf("invalid discord host %q: must be discord.com", host)
	}
	return nil
}

// NewSafeHTTPClient creates an HTTP client with socket-level IP validation against
// SSRF and DNS rebinding, plus redirect validation.
func NewSafeHTTPClient(allowPrivateHosts bool, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if !allowPrivateHosts {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve %q: %w", host, err)
			}
			var lastErr error
			for _, ip := range ips {
				if err := checkIP(ip.IP); err != nil {
					lastErr = fmt.Errorf("host %s resolved to blocked address: %w", host, err)
					continue
				}
				target := net.JoinHostPort(ip.IP.String(), port)
				conn, err := dialer.DialContext(ctx, network, target)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("no allowed addresses for %s", host)
		}
	} else {
		transport.DialContext = dialer.DialContext
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if !allowPrivateHosts {
				if err := checkHost(req.Context(), req.URL.Host); err != nil {
					return fmt.Errorf("redirect blocked: %w", err)
				}
			}
			return nil
		},
	}
}
