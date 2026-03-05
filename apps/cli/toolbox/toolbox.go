// Copyright Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package toolbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/daytonaio/daytona/cli/config"
	apiclient "github.com/daytonaio/daytona/libs/api-client-go"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

type ExecuteRequest struct {
	Command string   `json:"command"`
	Cwd     *string  `json:"cwd,omitempty"`
	Timeout *float32 `json:"timeout,omitempty"`
	TTY     *bool    `json:"tty,omitempty"`
}

type ExecuteResponse struct {
	ExitCode float32 `json:"exitCode"`
	Result   string  `json:"result"`
}

type ExecuteTTYRequest struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Cwd     *string           `json:"cwd,omitempty"`
	Timeout *uint32           `json:"timeout,omitempty"`
	Cols    *uint16           `json:"cols,omitempty"`
	Rows    *uint16           `json:"rows,omitempty"`
	Envs    map[string]string `json:"envs,omitempty"`
}

type ExecuteTTYResponse struct {
	SessionID string `json:"sessionId"`
}

type Client struct {
	apiClient *apiclient.APIClient
}

func NewClient(apiClient *apiclient.APIClient) *Client {
	return &Client{
		apiClient: apiClient,
	}
}

// Gets the toolbox proxy URL for a sandbox, caching by region in config
func (c *Client) getProxyURL(ctx context.Context, sandboxId, region string) (string, error) {
	// Check config cache first
	cachedURL, err := config.GetToolboxProxyUrl(region)
	if err == nil && cachedURL != "" {
		return cachedURL, nil
	}

	// Fetch from API
	toolboxProxyUrl, _, err := c.apiClient.SandboxAPI.GetToolboxProxyUrl(ctx, sandboxId).Execute()
	if err != nil {
		return "", fmt.Errorf("failed to get toolbox proxy URL: %w", err)
	}

	// Best-effort caching
	_ = config.SetToolboxProxyUrl(region, toolboxProxyUrl.Url)

	return toolboxProxyUrl.Url, nil
}

func (c *Client) ExecuteCommand(ctx context.Context, sandbox *apiclient.Sandbox, request ExecuteRequest) (*ExecuteResponse, error) {
	proxyURL, err := c.getProxyURL(ctx, sandbox.Id, sandbox.Target)
	if err != nil {
		return nil, err
	}

	return c.executeCommandViaProxy(ctx, proxyURL, sandbox.Id, request)
}

// TODO: replace this with the toolbox api client at some point
func (c *Client) executeCommandViaProxy(ctx context.Context, proxyURL, sandboxId string, request ExecuteRequest) (*ExecuteResponse, error) {
	// Build the URL: {proxyUrl}/{sandboxId}/process/execute
	url := fmt.Sprintf("%s/%s/process/execute", strings.TrimSuffix(proxyURL, "/"), sandboxId)

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	activeProfile, err := cfg.GetActiveProfile()
	if err != nil {
		return nil, err
	}

	if activeProfile.Api.Key != nil {
		req.Header.Set("Authorization", "Bearer "+*activeProfile.Api.Key)
	} else if activeProfile.Api.Token != nil {
		req.Header.Set("Authorization", "Bearer "+activeProfile.Api.Token.AccessToken)
	}

	if activeProfile.ActiveOrganizationId != nil {
		req.Header.Set("X-Daytona-Organization-ID", *activeProfile.ActiveOrganizationId)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var response ExecuteResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// ExecuteCommandTTY creates a TTY execution session and handles interactive communication
func (c *Client) ExecuteCommandTTY(ctx context.Context, sandbox *apiclient.Sandbox, request ExecuteTTYRequest) error {
	proxyURL, err := c.getProxyURL(ctx, sandbox.Id, sandbox.Target)
	if err != nil {
		return err
	}

	// Create TTY session
	sessionID, err := c.createTTYSession(ctx, proxyURL, sandbox.Id, request)
	if err != nil {
		return err
	}

	// Connect to the session as an interactive terminal
	return c.connectAndStreamTTY(ctx, proxyURL, sandbox.Id, sessionID)
}

// createTTYSession creates a new TTY execution session
func (c *Client) createTTYSession(ctx context.Context, proxyURL, sandboxId string, request ExecuteTTYRequest) (string, error) {
	url := fmt.Sprintf("%s/%s/process/execute-tty", strings.TrimSuffix(proxyURL, "/"), sandboxId)

	body, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	cfg, err := config.GetConfig()
	if err != nil {
		return "", err
	}

	activeProfile, err := cfg.GetActiveProfile()
	if err != nil {
		return "", err
	}

	if activeProfile.Api.Key != nil {
		req.Header.Set("Authorization", "Bearer "+*activeProfile.Api.Key)
	} else if activeProfile.Api.Token != nil {
		req.Header.Set("Authorization", "Bearer "+activeProfile.Api.Token.AccessToken)
	}

	if activeProfile.ActiveOrganizationId != nil {
		req.Header.Set("X-Daytona-Organization-ID", *activeProfile.ActiveOrganizationId)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create TTY session: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create TTY session: status %d: %s", resp.StatusCode, string(respBody))
	}

	var response ExecuteTTYResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", fmt.Errorf("failed to parse TTY session response: %w", err)
	}

	return response.SessionID, nil
}

// connectAndStreamTTY connects to a TTY session via WebSocket and streams stdin/stdout/stderr
func (c *Client) connectAndStreamTTY(ctx context.Context, proxyURL, sandboxId, sessionID string) error {
	baseURL := strings.TrimSuffix(proxyURL, "/")
	// Convert http(s) URL to ws(s) URL
	wsProto := "ws"
	if strings.HasPrefix(baseURL, "https") {
		wsProto = "wss"
		baseURL = baseURL[5:] // Remove "https"
	} else if strings.HasPrefix(baseURL, "http") {
		baseURL = baseURL[4:] // Remove "http"
	}
	baseURL = strings.TrimPrefix(baseURL, "://")

	wsURL := fmt.Sprintf("%s://%s/%s/process/execute-tty/%s", wsProto, baseURL, sandboxId, sessionID)

	cfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	activeProfile, err := cfg.GetActiveProfile()
	if err != nil {
		return err
	}

	// Dial WebSocket
	header := http.Header{}
	if activeProfile.Api.Key != nil {
		header.Set("Authorization", "Bearer "+*activeProfile.Api.Key)
	} else if activeProfile.Api.Token != nil {
		header.Set("Authorization", "Bearer "+activeProfile.Api.Token.AccessToken)
	}
	if activeProfile.ActiveOrganizationId != nil {
		header.Set("X-Daytona-Organization-ID", *activeProfile.ActiveOrganizationId)
	}

	dialer := websocket.Dialer{}
	ws, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("failed to connect to TTY session: %w", err)
	}
	defer ws.Close()

	// Set up raw terminal mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to setup terminal: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Get initial terminal size and send to server
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		cols = 80
		rows = 24
	}
	c.resizeTTYSession(ctx, proxyURL, sandboxId, sessionID, uint16(cols), uint16(rows))

	// Handle terminal resizing
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)
	defer signal.Stop(sigChan)

	done := make(chan error, 1)

	go func() {
		for range sigChan {
			cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
			if err == nil {
				c.resizeTTYSession(ctx, proxyURL, sandboxId, sessionID, uint16(cols), uint16(rows))
			}
		}
	}()

	// Read from stdin and write to WebSocket
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buffer)
			if err != nil {
				if err != io.EOF {
					done <- err
				}
				return
			}
			if n > 0 {
				ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := ws.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
					done <- err
					return
				}
			}
		}
	}()

	// Read from WebSocket and write to stdout
	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				if !websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					return
				}
				done <- err
				return
			}

			// Check if it's a control message
			if isControlMessage(data) {
				handleControlMessage(data)
				return
			}

			// Write to stdout
			os.Stdout.Write(data)
		}
	}()

	// Wait for either i/o to fail
	err = <-done
	return err
}

// resizeTTYSession sends a resize request to the TTY session
func (c *Client) resizeTTYSession(ctx context.Context, proxyURL, sandboxId, sessionID string, cols, rows uint16) error {
	url := fmt.Sprintf("%s/%s/process/execute-tty/%s/resize", strings.TrimSuffix(proxyURL, "/"), sandboxId, sessionID)

	req := map[string]uint16{
		"cols": cols,
		"rows": rows,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	cfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	activeProfile, err := cfg.GetActiveProfile()
	if err != nil {
		return err
	}

	if activeProfile.Api.Key != nil {
		httpReq.Header.Set("Authorization", "Bearer "+*activeProfile.Api.Key)
	} else if activeProfile.Api.Token != nil {
		httpReq.Header.Set("Authorization", "Bearer "+activeProfile.Api.Token.AccessToken)
	}

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

// Helper functions

func getHost(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	return u.Host
}

func isControlMessage(data []byte) bool {
	var msg map[string]interface{}
	return json.Unmarshal(data, &msg) == nil && msg["type"] == "control"
}

func handleControlMessage(data []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	if status, ok := msg["status"].(string); ok {
		if status == "exited" {
			if exitCode, ok := msg["exitCode"].(float64); ok {
				os.Exit(int(exitCode))
			}
			os.Exit(0)
		}
	}
}
