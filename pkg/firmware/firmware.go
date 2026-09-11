// Package firmware checks Flydigi's firmware update service for newer
// controller firmware. It mirrors the request made by Flydigi Space Station 4.
package firmware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultAPIBase is the base URL used by Flydigi Space Station 4.
	DefaultAPIBase = "https://api.flydigi.com/pc"
	// DefaultAppVersion is the Space Station version we impersonate.
	DefaultAppVersion = "4.2.2.3"
)

// Chip describes an available firmware image for one chip of the controller.
type Chip struct {
	Version       string `json:"version"`
	URL           string `json:"url"`
	Info          string `json:"info"`
	MinAppVersion string `json:"min_app_version"`
	IsPush        int    `json:"is_push"`
}

// Response is the parsed reply of the update service.
type Response struct {
	DeviceCode string           `json:"device_code"`
	Chips      map[string]*Chip `json:"chip_list"`
}

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Request describes the controller to check.
type Request struct {
	DeviceCode string // e.g. "f5"
	DeviceID   int32  // e.g. 144
	Wireless   bool   // check the dongle firmware instead of the main chip

	// Current versions. Leave empty to ask for the latest available version.
	MainVersion   string
	DongleVersion string

	APIBase    string
	AppVersion string
}

// Check queries the update service.
func Check(ctx context.Context, req Request) (*Response, error) {
	if req.APIBase == "" {
		req.APIBase = DefaultAPIBase
	}
	if req.AppVersion == "" {
		req.AppVersion = DefaultAppVersion
	}
	if req.DeviceCode == "" {
		return nil, fmt.Errorf("unknown device code for device id %d", req.DeviceID)
	}

	body := map[string]any{
		"device_code": req.DeviceCode,
		"device_id":   req.DeviceID,
		"app_version": req.AppVersion,
	}
	if req.Wireless {
		body["dongle_chip"] = req.DongleVersion
	} else {
		body["main_chip"] = req.MainVersion
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(req.APIBase, "/") + "/Update/firmware"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("appversion", req.AppVersion)

	client := &http.Client{Timeout: 20 * time.Second}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request update service: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update service returned HTTP %d", resp.StatusCode)
	}

	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if env.Code != 0 && env.Code != 200 {
		return nil, fmt.Errorf("update service error %d: %s", env.Code, env.Message)
	}

	var out Response
	if err := json.Unmarshal(env.Data, &out); err != nil {
		return nil, fmt.Errorf("decode response data: %w", err)
	}

	return &out, nil
}

// CompareVersions compares dotted numeric versions ("7.1.5.4"). It returns
// -1, 0 or 1 like strings.Compare.
func CompareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")

	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}

	return 0
}
