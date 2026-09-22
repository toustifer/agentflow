package hub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeviceLoginStart is step 1 of browser device authorization.
// It needs no prior credential — that is the point of the flow.
type DeviceLoginStart struct {
	Code            string `json:"code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
}

// StartDeviceLogin POSTs /v1/hub/auth/device and returns the user code plus the
// URL the human must open. Soft on transport errors.
//
// baseURL empty falls back to DefaultBaseURL.
func StartDeviceLogin(ctx context.Context, baseURL string) (DeviceLoginStart, Result) {
	const op = "login_start"
	if killSwitchOn() {
		return DeviceLoginStart{}, Result{Status: StatusDisabled, Op: op, Message: "HUB_SYNC/HUB_ENABLED off"}
	}
	baseURL = resolveBaseURL(baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/hub/auth/device", nil)
	if err != nil {
		return DeviceLoginStart{}, Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: defaultTimeout}).Do(req)
	if err != nil {
		return DeviceLoginStart{}, Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode >= 300 {
		return DeviceLoginStart{}, Result{
			Status: StatusFailed, Op: op, Code: resp.StatusCode,
			Message: truncateMsg(string(body)),
		}
	}
	start, ok := parseDeviceStart(body)
	if !ok {
		return DeviceLoginStart{}, Result{Status: StatusFailed, Op: op, Message: "bad device response"}
	}
	if start.VerificationURL == "" {
		start.VerificationURL = baseURL + "/auth/device?code=" + url.QueryEscape(start.Code)
	}
	return start, Result{Status: StatusOK, Op: op, Message: start.Code}
}

// parseDeviceStart accepts both the {data:{...}} envelope and a flat body.
func parseDeviceStart(body []byte) (DeviceLoginStart, bool) {
	var wrap struct {
		Data DeviceLoginStart `json:"data"`
	}
	if json.Unmarshal(body, &wrap) == nil && strings.TrimSpace(wrap.Data.Code) != "" {
		return wrap.Data, true
	}
	var flat DeviceLoginStart
	if json.Unmarshal(body, &flat) == nil && strings.TrimSpace(flat.Code) != "" {
		return flat, true
	}
	return DeviceLoginStart{}, false
}

// FinishDeviceLogin polls GET /v1/hub/auth/device/token?code=... once.
//
// 202 (or a {"status":"pending"} payload) means the human has not approved yet:
// that is StatusSkipped, not an error — the caller should poll again.
// On success the JWT is persisted to ~/.agent-hub/config.json.
func FinishDeviceLogin(ctx context.Context, baseURL, code string) (token string, r Result) {
	const op = "login_finish"
	code = strings.TrimSpace(code)
	if code == "" {
		return "", Result{Status: StatusFailed, Op: op, Message: "code required"}
	}
	if killSwitchOn() {
		return "", Result{Status: StatusDisabled, Op: op, Message: "HUB_SYNC/HUB_ENABLED off"}
	}
	baseURL = resolveBaseURL(baseURL)

	target := baseURL + "/v1/hub/auth/device/token?code=" + url.QueryEscape(code)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: defaultTimeout}).Do(req)
	if err != nil {
		return "", Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode == http.StatusAccepted {
		return "", Result{Status: StatusSkipped, Op: op, Message: pendingMessage}
	}
	if resp.StatusCode >= 300 {
		return "", Result{Status: StatusFailed, Op: op, Code: resp.StatusCode, Message: truncateMsg(string(body))}
	}

	var wrap struct {
		Data struct {
			Token  string `json:"token"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &wrap) != nil {
		return "", Result{Status: StatusFailed, Op: op, Message: "bad token response"}
	}
	if wrap.Data.Status == "pending" || strings.TrimSpace(wrap.Data.Token) == "" {
		return "", Result{Status: StatusSkipped, Op: op, Message: pendingMessage}
	}
	token = wrap.Data.Token
	if err := SaveHomeConfig(homeConfigFile{
		BaseURL: baseURL,
		HubURL:  baseURL,
		Token:   token,
		LoginAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		// The token is still usable for this process; report the persistence failure.
		return token, Result{Status: StatusFailed, Op: op, Message: "token ok but save config: " + err.Error()}
	}
	return token, Result{Status: StatusOK, Op: op, Message: "token saved to ~/.agent-hub/config.json"}
}

const pendingMessage = "pending approval — open verification URL and click Approve"

func resolveBaseURL(baseURL string) string {
	return strings.TrimRight(firstNonEmpty(baseURL, DefaultBaseURL), "/")
}
