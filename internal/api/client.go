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

var (
	ErrUnauthorized           = errors.New("unauthorized")
	ErrPasswordChangeRequired = errors.New("password change required")
)

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
		Configured  bool   `json:"configured"`
		State       string `json:"state"`
		Players     int    `json:"players"`
		MaxPlayers  int    `json:"max_players"`
		Version     string `json:"version"`
		JavaPort    int    `json:"java_port"`
		Bedrock     bool   `json:"bedrock"`
		BedrockPort int    `json:"bedrock_port"`
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

type Players struct {
	Configured bool     `json:"configured"`
	State      string   `json:"state"`
	Online     int      `json:"online"`
	Max        int      `json:"max"`
	Names      []string `json:"names"`
	Error      string   `json:"error,omitempty"`
}

type MinecraftActionResponse struct {
	OK                   bool     `json:"ok"`
	Action               string   `json:"action,omitempty"`
	Message              string   `json:"message,omitempty"`
	ConfirmationRequired bool     `json:"confirmation_required,omitempty"`
	Reason               string   `json:"reason,omitempty"`
	Players              []string `json:"players,omitempty"`
	Online               int      `json:"online,omitempty"`
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

func (c *Client) ChangePassword(ctx context.Context, session, currentPassword, newPassword string) error {
	return c.do(ctx, http.MethodPost, "/v1/auth/password", session, map[string]string{
		"current_password": currentPassword,
		"new_password":     newPassword,
	}, nil)
}

func (c *Client) Status(ctx context.Context, session string) (Status, error) {
	var out Status
	err := c.do(ctx, http.MethodGet, "/v1/status", session, nil, &out)
	return out, err
}

func (c *Client) Players(ctx context.Context, session string) (Players, error) {
	var out Players
	err := c.do(ctx, http.MethodGet, "/v1/players", session, nil, &out)
	return out, err
}

func (c *Client) MinecraftAction(ctx context.Context, session, action string, confirmPlayers bool) (MinecraftActionResponse, error) {
	path, err := minecraftActionPath(action)
	if err != nil {
		return MinecraftActionResponse{}, err
	}
	body := map[string]bool{"confirm_players": confirmPlayers}
	return c.doMinecraftAction(ctx, path, session, body)
}

func minecraftActionPath(action string) (string, error) {
	switch action {
	case "start", "stop", "restart":
		return "/v1/minecraft/" + action, nil
	default:
		return "", fmt.Errorf("unsupported Minecraft action %q", action)
	}
}

func (c *Client) doMinecraftAction(ctx context.Context, path, session string, body any) (MinecraftActionResponse, error) {
	var out MinecraftActionResponse
	data, err := json.Marshal(body)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+path, bytes.NewReader(data))
	if err != nil {
		return out, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+session)
	resp, err := c.http.Do(req)
	if err != nil {
		return out, fmt.Errorf("management API unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return out, ErrUnauthorized
	}
	if resp.StatusCode == http.StatusForbidden {
		return out, ErrPasswordChangeRequired
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&out); err != nil {
		return out, fmt.Errorf("management API returned invalid action response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if out.Message != "" {
			return out, errors.New(out.Message)
		}
		return out, fmt.Errorf("management API %s", resp.Status)
	}
	return out, nil
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
	if resp.StatusCode == http.StatusForbidden {
		return ErrPasswordChangeRequired
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
