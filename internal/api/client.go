package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

var ErrUnauthorized = errors.New("unauthorized")

type Client struct {
	http *http.Client
}

type LoginResponse struct {
	Session       string `json:"session"`
	MustChange    bool   `json:"must_change"`
	ManagementAPI string `json:"management_api"`
}

type Status struct {
	System struct {
		Variant string `json:"variant"`
		Health  string `json:"health"`
		IPv4    string `json:"ipv4"`
	} `json:"system"`
	Minecraft struct {
		State      string `json:"state"`
		Players    int    `json:"players"`
		MaxPlayers int    `json:"max_players"`
		Version    string `json:"version"`
		JavaPort   int    `json:"java_port"`
		Bedrock    bool   `json:"bedrock"`
		BedrockPort int   `json:"bedrock_port"`
	} `json:"minecraft"`
	Backup struct {
		Enabled bool   `json:"enabled"`
		Last    string `json:"last"`
		Next    string `json:"next"`
	} `json:"backup"`
	JustVoxel struct {
		Version string `json:"version"`
		WebUI   string `json:"webui"`
		API     string `json:"management_api"`
	} `json:"justvoxel"`
}

func NewClient(socket string) *Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: 10 * time.Second}}
}

func (c *Client) Login(ctx context.Context, username, password string) (LoginResponse, error) {
	var out LoginResponse
	err := c.do(ctx, http.MethodPost, "/v1/auth/login", "", map[string]string{"username": username, "password": password}, &out)
	return out, err
}

func (c *Client) Logout(ctx context.Context, session string) error {
	return c.do(ctx, http.MethodPost, "/v1/auth/logout", session, nil, nil)
}

func (c *Client) Status(ctx context.Context, session string) (Status, error) {
	var out Status
	err := c.do(ctx, http.MethodGet, "/v1/status", session, nil, &out)
	return out, err
}

func (c *Client) do(ctx context.Context, method, path, session string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("management API unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("management API %s: %s", resp.Status, string(limited))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return err
		}
	}
	return nil
}
