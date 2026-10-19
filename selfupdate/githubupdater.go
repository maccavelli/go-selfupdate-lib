package selfupdate

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// The one-call common path
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D5,
// Amendment A1).

// GitHubUpdaterOptions configure NewGitHubUpdater. Repository, Product and
// Version are required; every other field has a default.
type GitHubUpdaterOptions struct {
	// Repository is the GitHub owner/name pair. Required.
	Repository Repository
	// Product is the asset prefix, as Request.Product names it. Required.
	Product string
	// Version is the running version, for the User-Agent. Required.
	Version string
	// Platforms are the platforms the program ships. Nil means the running
	// platform only.
	Platforms []Platform
	// APIBaseURL is the GitHub API origin. Nil means https://api.github.com.
	APIBaseURL *url.URL
	// Client is the HTTP client. Nil means a client with transport timeouts
	// (30 s to dial, 10 s for TLS, 30 s for response headers) and no
	// Client.Timeout, so a slow large download is bounded by the context
	// rather than cut off.
	Client *http.Client
	// Stdin is the terminal the confirmer reads. Nil means os.Stdin.
	Stdin *os.File
	// Stderr receives the text reporter's lines and the prompt. Nil means
	// os.Stderr.
	Stderr io.Writer
}

// NewGitHubUpdater returns an Updater for a standalone program released on
// GitHub with exact asset names, composed from this package's own parts
// with their safe defaults: DefaultLimits, NewStrictVersionPolicy,
// NewExactAssetSelector, a StandaloneInstaller with InstallOptions{}, a
// text reporter and a terminal confirmer. New fills no defaults; use it,
// with the same parts, to change any of them.
func NewGitHubUpdater(o GitHubUpdaterOptions) (*Updater, error) {
	if err := validateProduct(o.Product); err != nil {
		return nil, fmt.Errorf("selfupdate: github updater: %w", err)
	}
	if o.Version == "" {
		return nil, errors.New("selfupdate: github updater: version is required")
	}
	client := o.Client
	if client == nil {
		client = defaultHTTPClient()
	}
	stdin, stderr := o.Stdin, o.Stderr
	if stdin == nil {
		stdin = os.Stdin
	}
	if isNil(stderr) {
		stderr = os.Stderr
	}
	platforms := o.Platforms
	if platforms == nil {
		platforms = []Platform{runningPlatform()}
	}
	src, err := NewGitHubSource(GitHubOptions{
		Repository: o.Repository,
		Client:     client,
		APIBaseURL: o.APIBaseURL,
		UserAgent:  UserAgent(o.Product, o.Version),
		Limits:     DefaultLimits(),
	})
	if err != nil {
		return nil, fmt.Errorf("selfupdate: github updater: %w", err)
	}
	assets, err := NewExactAssetSelector(platforms)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: github updater: %w", err)
	}
	installer, err := NewStandaloneInstaller(InstallOptions{})
	if err != nil {
		return nil, fmt.Errorf("selfupdate: github updater: %w", err)
	}
	return New(Config{
		Source:    src,
		Versions:  NewStrictVersionPolicy(),
		Assets:    assets,
		Installer: installer,
		Reporter:  NewTextReporter(stderr),
		Confirmer: NewTerminalConfirmer(stdin, stderr),
		Limits:    DefaultLimits(),
	})
}

// defaultHTTPClient is NewGitHubUpdater's client: http.DefaultTransport's
// settings, or Go 1.27.2's when a program replaced it with another
// RoundTripper, plus a response-header timeout, and no Client.Timeout.
func defaultHTTPClient() *http.Client {
	t, ok := http.DefaultTransport.(*http.Transport)
	if ok {
		t = t.Clone()
	} else {
		t = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		}
	}
	t.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: t}
}
