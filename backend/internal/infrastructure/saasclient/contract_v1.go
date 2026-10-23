package saasclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

const contractPath = "/api/v1/provisioning"
const maxResponseBytes = 1 << 20

type ContractV1Client struct {
	httpClient *http.Client
}

func NewContractV1Client(client *http.Client) *ContractV1Client {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	clientCopy := *client
	// A API key não pode acompanhar um redirecionamento para outra origem.
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &ContractV1Client{httpClient: &clientCopy}
}

func (c *ContractV1Client) Discover(ctx context.Context, baseURL, apiKey string) (domain.Capabilities, error) {
	var capabilities domain.Capabilities
	if err := c.request(ctx, baseURL, apiKey, http.MethodGet, nil, "", &capabilities); err != nil {
		return domain.Capabilities{}, err
	}
	if capabilities.ContractVersion != "1" {
		return domain.Capabilities{}, domain.ErrUnsupportedContractVersion
	}
	return capabilities, nil
}

func (c *ContractV1Client) Execute(ctx context.Context, baseURL, apiKey string, command domain.Command, idempotencyKey string) (domain.Response, error) {
	var response domain.Response
	if err := c.request(ctx, baseURL, apiKey, http.MethodPost, command, idempotencyKey, &response); err != nil {
		return domain.Response{}, err
	}
	if response.ContractVersion != "1" {
		return domain.Response{}, domain.ErrUnsupportedContractVersion
	}
	return response, nil
}

func (c *ContractV1Client) request(ctx context.Context, baseURL, apiKey, method string, body any, idempotencyKey string, result any) error {
	endpoint, err := contractURL(baseURL)
	if err != nil {
		return err
	}
	var requestBody io.Reader
	if body != nil {
		encoded, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return fmt.Errorf("encode SaaS contract request: %w", marshalErr)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return fmt.Errorf("create SaaS contract request: %w", err)
	}
	request.Header.Set("X-Lynx-API-Key", apiKey)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("SaaS contract request failed: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	responseBytes, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read SaaS contract response: %w", err)
	}
	if len(responseBytes) > maxResponseBytes {
		return errors.New("SaaS contract response exceeded the size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return remoteError(response.StatusCode, responseBytes)
	}
	if err := json.Unmarshal(responseBytes, result); err != nil {
		return fmt.Errorf("decode SaaS contract response: %w", err)
	}
	return nil
}

func contractURL(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("SaaS URL must be an absolute origin without credentials or query parameters")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("SaaS URL must contain only the origin")
	}
	if parsed.Scheme == "https" {
		if ip := net.ParseIP(parsed.Hostname()); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
			return "", errors.New("private IP addresses are not valid SaaS origins")
		}
	} else if parsed.Scheme != "http" || !isLoopbackName(parsed.Hostname()) {
		return "", errors.New("SaaS URLs must use HTTPS; HTTP is allowed only for local development")
	}
	return strings.TrimRight(baseURL, "/") + contractPath, nil
}

func isLoopbackName(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func remoteError(status int, body []byte) error {
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &response)
	return &domain.RemoteError{StatusCode: status, Code: response.Error.Code, Message: response.Error.Message}
}
