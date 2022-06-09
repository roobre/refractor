package client

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	HTTPClient *http.Client
	baseUrl    string
}

type Request struct {
	ResponseChan chan Response
	Path         string
}

type Response struct {
	HTTPResponse *http.Response
	Error        error
	Done         func()
}

func NewClient(baseUrl string) *Client {
	return &Client{
		HTTPClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
		baseUrl: baseUrl,
	}
}

func (c *Client) String() string {
	return c.baseUrl
}

func (c *Client) Do(path string) (r Response) {
	// TODO: Calculate a better deadline by making a HEAD request and a target throughput
	//ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(2*time.Second))
	//defer cancel()

	url := strings.TrimSuffix(c.baseUrl, "/")
	url += "/"
	url += strings.TrimPrefix(path, "/")
	//req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		r.Error = fmt.Errorf("building request to %s: %w", url, err)
		return
	}

	log.Infof("%s %s", req.Method, req.URL.String())
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		r.Error = fmt.Errorf("performing %s to %q: %w", req.Method, req.URL.String(), err)
		return
	}

	resp.Header.Add("X-refractored-By", c.String())
	r.HTTPResponse = resp

	return
}
