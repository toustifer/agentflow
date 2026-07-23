package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DeviceLoginStart is step 1 of browser device auth (no prior credentials required).
type DeviceLoginStart struct {
	Code            string `json:"code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
}

// StartDeviceLogin POSTs /v1/hub/auth/device. Soft on network errors.
// baseURL empty → default production hub.
func StartDeviceLogin(ctx context.Context, baseURL string) (DeviceLoginStart, Result) {
	const op = "login_start"
	baseURL = strings.TrimRight(firstNonEmpty(baseURL, "https://hub.stifer.xyz"), "/")
	if killSwitchOn() {
		return DeviceLoginStart{}, Result{Status: StatusDisabled, Op: op, Message: "HUB_SYNC/HUB_ENABLED off"}
	}
	url := baseURL + "/v1/hub/auth/device"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return DeviceLoginStart{}, Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	client := &http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return DeviceLoginStart{}, Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return DeviceLoginStart{}, Result{
			Status: StatusFailed, Op: op, Code: resp.StatusCode,
			Message: truncateMsg(string(body)),
		}
	}
	var wrap struct {
		Data DeviceLoginStart `json:"data"`
	}
	if json.Unmarshal(body, &wrap) != nil || wrap.Data.Code == "" {
		// some builds return flat
		var flat DeviceLoginStart
		if json.Unmarshal(body, &flat) == nil && flat.Code != "" {
			wrap.Data = flat
		} else {
			return DeviceLoginStart{}, Result{Status: StatusFailed, Op: op, Message: "bad device response"}
		}
	}
	if wrap.Data.VerificationURL == "" {
		wrap.Data.VerificationURL = baseURL + "/auth/device?code=" + wrap.Data.Code
	}
	return wrap.Data, Result{Status: StatusOK, Op: op, Message: wrap.Data.Code}
}

// FinishDeviceLogin exchanges approved device code for JWT and persists to ~/.agent-hub/config.json.
func FinishDeviceLogin(ctx context.Context, baseURL, code string) (token string, r Result) {
	const op = "login_finish"
	code = strings.TrimSpace(code)
	if code == "" {
		return "", Result{Status: StatusFailed, Op: op, Message: "code required"}
	}
	if killSwitchOn() {
		return "", Result{Status: StatusDisabled, Op: op, Message: "HUB_SYNC/HUB_ENABLED off"}
	}
	baseURL = strings.TrimRight(firstNonEmpty(baseURL, "https://hub.stifer.xyz"), "/")
	url := baseURL + "/v1/hub/auth/device/token?code=" + urlQueryEscape(code)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	client := &http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusAccepted {
		return "", Result{Status: StatusSkipped, Op: op, Message: "pending approval — open verification URL and click Approve"}
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
	if wrap.Data.Status == "pending" || wrap.Data.Token == "" {
		return "", Result{Status: StatusSkipped, Op: op, Message: "pending approval — open verification URL and click Approve"}
	}
	token = wrap.Data.Token
	if err := SaveHomeConfig(homeConfigFile{
		BaseURL: baseURL,
		HubURL:  baseURL,
		Token:   token,
		LoginAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return token, Result{Status: StatusFailed, Op: op, Message: "token ok but save config: " + err.Error()}
	}
	return token, Result{Status: StatusOK, Op: op, Message: "token saved to ~/.agent-hub/config.json"}
}

// urlQueryEscape is a tiny escape for device codes (alphanumeric).
func urlQueryEscape(s string) string {
	// device codes are A-Z0-9; still escape anything unexpected
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return b.String()
}

func truncateMsg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
