package client

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kowerkoint/partner-watch/desktop/internal/state"
)

type Client struct {
	origin     *url.URL
	credential string
	http       *http.Client
}

func New(origin, credential string) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || (u.Path != "" && u.Path != "/") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("server_url must be an HTTPS origin")
	}
	u.Path = ""
	return &Client{u, credential, &http.Client{Timeout: 15 * time.Second}}, nil
}

func Enroll(ctx context.Context, origin, invite, name string) (state.State, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return state.State{}, err
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return state.State{}, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return state.State{}, err
	}
	body := map[string]any{"invitationToken": invite, "deviceName": name, "publicKey": base64.RawURLEncoding.EncodeToString(pub), "platform": "LINUX", "capabilities": []string{"notification.send", "capture"}}
	data, _ := json.Marshal(body)
	c, err := New(origin, "")
	if err != nil {
		return state.State{}, err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.resolve("v1/enrollments"), bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return state.State{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return state.State{}, fmt.Errorf("enrollment rejected (%d)", resp.StatusCode)
	}
	var out struct {
		DeviceID, PairID, Credential string
		Slot                         int
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&out) != nil || out.Credential == "" {
		return state.State{}, errors.New("invalid enrollment response")
	}
	return state.State{DeviceID: out.DeviceID, PairID: out.PairID, Slot: out.Slot, Credential: out.Credential, PrivateKey: base64.RawURLEncoding.EncodeToString(private)}, nil
}

func (c *Client) ForwardNotification(ctx context.Context, appName, desktopEntry, title, body string, postedAt time.Time) error {
	payload := map[string]any{"sourcePackage": firstNonempty(desktopEntry, "linux.desktop"), "sourceAppName": firstNonempty(appName, "Linux"), "title": title, "body": body, "postedAt": postedAt.UTC().Format(time.RFC3339Nano)}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.resolve("v1/forwarded-notifications"), bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.credential)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("notification rejected (%d)", resp.StatusCode)
	}
	return nil
}
func (c *Client) resolve(path string) string {
	return strings.TrimRight(c.origin.String(), "/") + "/" + path
}
func firstNonempty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
