package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/providers/dns/internal/errutils"
)

const defaultBaseURL = "https://api.reg.ru/api/regru2/"

// Client is the reg.ru client.
type Client struct {
	username   string
	password   string
	baseURL    *url.URL
	HTTPClient *http.Client
}

// NewClient creates a reg.ru client.
func NewClient(username, password string) *Client {
	baseURL, _ := url.Parse(defaultBaseURL)
	return &Client{
		username:   username,
		password:   password,
		baseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// AddTXTRecord adds a TXT record via "zone/add_txt".
func (c Client) AddTXTRecord(ctx context.Context, domain, subDomain, content string) error {
	formData := url.Values{}
	formData.Set("username", c.username)
	formData.Set("password", c.password)
	formData.Set("domain_name", domain)
	formData.Set("subdomain", subDomain)
	formData.Set("text", content)

	apiResp, err := c.doFormRequest(ctx, formData, "zone", "add_txt")
	if err != nil {
		return err
	}
	return apiResp.HasError()
}

// RemoveTxtRecord removes a TXT record via "zone/remove_record".
func (c Client) RemoveTxtRecord(ctx context.Context, domain, subDomain, content string) error {
	formData := url.Values{}
	formData.Set("username", c.username)
	formData.Set("password", c.password)
	formData.Set("domain_name", domain)
	formData.Set("subdomain", subDomain)
	formData.Set("record_type", "TXT")
	formData.Set("content", content)

	apiResp, err := c.doFormRequest(ctx, formData, "zone", "remove_record")
	if err != nil {
		return err
	}
	return apiResp.HasError()
}

// doFormRequest is a helper to POST form-urlencoded data to the reg.ru API,
// parse JSON response into an APIResponse struct, and handle non-2xx status codes.
func (c Client) doFormRequest(ctx context.Context, formData url.Values, fragments ...string) (*APIResponse, error) {
	endpoint := c.baseURL.JoinPath(fragments...)

	// ----- LOG: Request Data -----
	log.Println("===== REG.RU REQUEST =====")
	log.Printf("Endpoint: %s\n", endpoint.String())
	log.Printf("Form data (BE CAREFUL: includes credentials!): %s\n", formData.Encode())
	log.Println("==========================")

	// Build the request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Perform HTTP request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, errutils.NewHTTPDoError(req, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errutils.NewReadResponseError(req, resp.StatusCode, err)
	}

	// ----- LOG: Response Data -----
	log.Println("===== REG.RU RESPONSE =====")
	log.Printf("Status code: %d\n", resp.StatusCode)
	log.Printf("Response body: %s\n", string(raw))
	log.Println("==========================")

	if resp.StatusCode/100 != 2 {
		return nil, parseError(req, resp, raw)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, errutils.NewUnmarshalError(req, resp.StatusCode, raw, err)
	}

	return &apiResp, nil
}

// parseError is used when Reg.ru returns a non-2xx HTTP status code.
func parseError(req *http.Request, resp *http.Response, raw []byte) error {
	var errAPI APIResponse
	if err := json.Unmarshal(raw, &errAPI); err != nil {
		// If we cannot parse it as JSON, wrap it as an unexpected status code error.
		return errutils.NewUnexpectedStatusCodeError(req, resp.StatusCode, raw)
	}
	return fmt.Errorf("status code: %d, %w", resp.StatusCode, errAPI)
}
