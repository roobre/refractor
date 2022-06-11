package client

import (
	"context"
	"fmt"
	"github.com/rs/dnscache"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"strings"
	"time"
)

const clientHeader = "X-Refracted-By"

type Client struct {
	HTTPClient *http.Client
	resolver   *dnscache.Resolver
	baseUrl    string
}

type Config struct {
	PreDownloadTimeout time.Duration `yaml:"preDownloadTimeout"`
	DownloadTimeout    time.Duration `yaml:"downloadTimeout"`
}

func (c Config) WithDefaults() Config {
	if c.PreDownloadTimeout == 0 {
		c.PreDownloadTimeout = 3 * time.Second
	}

	if c.DownloadTimeout == 0 {
		c.DownloadTimeout = 2 * time.Minute
	}

	return c
}

type Request struct {
	Path         string
	Header       http.Header
	ResponseChan chan Response
}

type Response struct {
	HTTPResponse *http.Response
	Worker       string
	Error        error
	Done         func(written int64)
}

func NewClient(c Config, baseUrl string) *Client {
	c = c.WithDefaults()

	timeoutDialer := &net.Dialer{
		Timeout: c.PreDownloadTimeout,
	}

	resolver := &dnscache.Resolver{}

	// Stolen from https://github.com/rs/dnscache
	dialContext := func(ctx context.Context, network string, addr string) (conn net.Conn, err error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("splitting host and port %q: %w", addr, err)
		}
		ips, err := resolver.LookupHost(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("looking up %q: %w", host, err)
		}
		for _, ip := range ips {
			conn, err = timeoutDialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
			if err == nil {
				break
			}
		}
		return
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialContext,
		MaxIdleConns:          10,
		ResponseHeaderTimeout: c.PreDownloadTimeout,
		IdleConnTimeout:       c.PreDownloadTimeout,
		TLSHandshakeTimeout:   c.PreDownloadTimeout,
	}

	return &Client{
		HTTPClient: &http.Client{
			Transport: transport,
			Timeout:   c.DownloadTimeout,
		},
		baseUrl:  baseUrl,
		resolver: resolver,
	}
}

func (c *Client) String() string {
	return c.baseUrl
}

func (c *Client) URL(path string) string {
	url := strings.TrimSuffix(c.baseUrl, "/")
	url += "/"
	url += strings.TrimPrefix(path, "/")

	return url
}

func (c *Client) Do(request Request) (r Response) {
	c.resolver.Refresh(true)

	// TODO: Calculate a better deadline by making a HEAD request and a target throughput
	//ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(2*time.Second))
	//defer cancel()

	url := c.URL(request.Path)

	//req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		r.Error = fmt.Errorf("building request to %s: %w", url, err)
		return
	}

	req.Header = request.Header
	log.Debugf("%s %s", req.Method, req.URL.String())
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		r.Error = fmt.Errorf("performing %s to %q: %w", req.Method, req.URL.String(), err)
		return
	}

	resp.Header.Add(clientHeader, c.String())
	r.HTTPResponse = resp

	return
}
