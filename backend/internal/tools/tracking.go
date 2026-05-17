// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TrackShipmentTool queries shipping carriers for real-time tracking status.
//
// Supported carriers:
//   - dhl        — DHL Express Tracking API (requires DHL-API-Key)
//   - fedex      — FedEx Track API v1 (requires client_id:client_secret key format)
//   - sf_express — SF Express Open Platform (requires app_id:app_key)
//   - yto        — YTO Express Open API (requires app_id:app_key)
//   - sto        — STO Express Open API (requires app_id:app_key)
//   - best       — Best Express Open API (requires app_id:app_secret)
//
// API keys are looked up at call time via the injected getKey function,
// which is wired in gateway_tools.go to the provider key vault. This
// means vault updates take effect without a restart.
type TrackShipmentTool struct {
	http   *http.Client
	getKey func(carrier string) string // returns "" when key is absent
}

// NewTrackShipmentTool returns a tracking tool that resolves API keys via getKey.
// Pass nil to get a key-less instance (useful for tests that exercise stubs).
func NewTrackShipmentTool(getKey func(carrier string) string) *TrackShipmentTool {
	if getKey == nil {
		getKey = func(string) string { return "" }
	}
	return &TrackShipmentTool{
		http:   &http.Client{Timeout: 15 * time.Second},
		getKey: getKey,
	}
}

func (t *TrackShipmentTool) Name() string { return "track_shipment" }

func (t *TrackShipmentTool) Description() string {
	return "Track a shipment by carrier and tracking number. " +
		"Supported carriers: dhl, fedex, sf_express (SF Express), yto (YTO Express), sto (STO Express), best (Best Express). " +
		"Returns current status, location, and delivery estimates."
}

func (t *TrackShipmentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"carrier": map[string]any{
				"type":        "string",
				"description": "Carrier code. One of: dhl, fedex, sf_express, yto, sto, best.",
				"enum":        []string{"dhl", "fedex", "sf_express", "yto", "sto", "best"},
			},
			"tracking_number": map[string]any{
				"type":        "string",
				"description": "The shipment tracking number or waybill number.",
			},
		},
		"required": []string{"carrier", "tracking_number"},
	}
}

func (t *TrackShipmentTool) Execute(ctx context.Context, args map[string]any) *Result {
	carrier, _ := args["carrier"].(string)
	carrier = strings.ToLower(strings.TrimSpace(carrier))
	trackingNumber, _ := args["tracking_number"].(string)
	trackingNumber = strings.TrimSpace(trackingNumber)

	if carrier == "" {
		return ErrorResult("carrier is required (dhl, fedex, sf_express, yto, sto, best)")
	}
	if trackingNumber == "" {
		return ErrorResult("tracking_number is required")
	}

	switch carrier {
	case "dhl":
		return t.trackDHL(ctx, trackingNumber)
	case "fedex":
		return t.trackFedEx(ctx, trackingNumber)
	case "sf_express":
		return t.trackSFExpress(ctx, trackingNumber)
	case "yto":
		return t.trackYTO(ctx, trackingNumber)
	case "sto":
		return t.trackSTO(ctx, trackingNumber)
	case "best":
		return t.trackBest(ctx, trackingNumber)
	default:
		return ErrorResult(fmt.Sprintf("unsupported carrier %q — supported: dhl, fedex, sf_express, yto, sto, best", carrier))
	}
}

// --- DHL ---

const dhlTrackURL = "https://api.dhl.com/track/shipments"

func (t *TrackShipmentTool) trackDHL(ctx context.Context, trackingNumber string) *Result {
	apiKey := t.getKey("dhl")
	if apiKey == "" {
		return TextResult("No API key configured for carrier dhl. Configure it in Settings → Provider Keys (category: tracking, provider: dhl).")
	}

	u := dhlTrackURL + "?trackingNumber=" + url.QueryEscape(trackingNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("dhl: failed to build request: %v", err))
	}
	req.Header.Set("DHL-API-Key", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("dhl: request failed: %v", err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrorResult("dhl: API key rejected (401/403). Verify the key in Settings → Provider Keys.")
	}
	if resp.StatusCode == http.StatusNotFound {
		return TextResult(fmt.Sprintf("dhl: tracking number %q not found. Verify the number is correct and the shipment has been registered.", trackingNumber))
	}
	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("dhl: unexpected HTTP %d: %s", resp.StatusCode, truncateErr(string(body), 300)))
	}

	return TextResult(formatDHLResponse(trackingNumber, body))
}

type dhlTrackResponse struct {
	Shipments []struct {
		ID     string `json:"id"`
		Status struct {
			Timestamp   string `json:"timestamp"`
			Location    struct{ Address struct{ AddressLocality string `json:"addressLocality"` } `json:"address"`} `json:"location"`
			Status      string `json:"status"`
			Description string `json:"description"`
		} `json:"status"`
		Events []struct {
			Timestamp   string `json:"timestamp"`
			Location    struct{ Address struct{ AddressLocality string `json:"addressLocality"` } `json:"address"`} `json:"location"`
			Description string `json:"description"`
		} `json:"events"`
		EstimatedTimeOfDelivery string `json:"estimatedTimeOfDelivery"`
	} `json:"shipments"`
}

func formatDHLResponse(trackingNumber string, body []byte) string {
	var resp dhlTrackResponse
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Shipments) == 0 {
		// Fall back to raw JSON if parsing fails
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "", "  ") == nil {
			return fmt.Sprintf("DHL tracking for %s:\n%s", trackingNumber, pretty.String())
		}
		return fmt.Sprintf("DHL tracking for %s:\n%s", trackingNumber, string(body))
	}

	s := resp.Shipments[0]
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# DHL Shipment: %s\n\n", trackingNumber))
	sb.WriteString(fmt.Sprintf("**Status**: %s\n", s.Status.Description))
	if s.Status.Location.Address.AddressLocality != "" {
		sb.WriteString(fmt.Sprintf("**Current Location**: %s\n", s.Status.Location.Address.AddressLocality))
	}
	sb.WriteString(fmt.Sprintf("**Last Updated**: %s\n", s.Status.Timestamp))
	if s.EstimatedTimeOfDelivery != "" {
		sb.WriteString(fmt.Sprintf("**Estimated Delivery**: %s\n", s.EstimatedTimeOfDelivery))
	}
	if len(s.Events) > 0 {
		sb.WriteString("\n## Tracking History\n")
		limit := 5
		if len(s.Events) < limit {
			limit = len(s.Events)
		}
		for i := 0; i < limit; i++ {
			ev := s.Events[i]
			loc := ev.Location.Address.AddressLocality
			if loc != "" {
				sb.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", ev.Timestamp, ev.Description, loc))
			} else {
				sb.WriteString(fmt.Sprintf("- **%s** — %s\n", ev.Timestamp, ev.Description))
			}
		}
	}
	return sb.String()
}

// --- FedEx ---

const (
	fedexTokenURL = "https://apis.fedex.com/oauth/token"
	fedexTrackURL = "https://apis.fedex.com/track/v1/trackingnumbers"
)

func (t *TrackShipmentTool) trackFedEx(ctx context.Context, trackingNumber string) *Result {
	rawKey := t.getKey("fedex")
	if rawKey == "" {
		return TextResult("No API key configured for carrier fedex. Configure it in Settings → Provider Keys (category: tracking, provider: fedex). Key format: client_id:client_secret")
	}

	// Key format is "client_id:client_secret"
	parts := strings.SplitN(rawKey, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ErrorResult("fedex: invalid key format. Expected client_id:client_secret (colon-separated).")
	}
	clientID, clientSecret := parts[0], parts[1]

	// Step 1: Get OAuth token
	bearerToken, err := t.fedexGetToken(ctx, clientID, clientSecret)
	if err != nil {
		return ErrorResult(fmt.Sprintf("fedex: auth failed: %v", err))
	}

	// Step 2: Track shipment
	return t.fedexTrack(ctx, bearerToken, trackingNumber)
}

func (t *TrackShipmentTool) fedexGetToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fedexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateErr(string(body), 200))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access_token in response")
	}
	return tokenResp.AccessToken, nil
}

func (t *TrackShipmentTool) fedexTrack(ctx context.Context, bearerToken, trackingNumber string) *Result {
	payload := map[string]any{
		"includeDetailedScans": true,
		"trackingInfo": []map[string]any{
			{
				"trackingNumberInfo": map[string]any{
					"trackingNumber": trackingNumber,
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fedexTrackURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ErrorResult(fmt.Sprintf("fedex: failed to build track request: %v", err))
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-locale", "en_US")

	resp, err := t.http.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("fedex: track request failed: %v", err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrorResult("fedex: bearer token rejected. The client_id/client_secret may be invalid or expired.")
	}
	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("fedex: unexpected HTTP %d: %s", resp.StatusCode, truncateErr(string(body), 300)))
	}

	return TextResult(formatFedExResponse(trackingNumber, body))
}

type fedexTrackResponse struct {
	Output struct {
		CompleteTrackResults []struct {
			TrackingInfo []struct {
				TrackingNumber string `json:"trackingNumber"`
			} `json:"trackingInfo"`
			TrackResults []struct {
				TrackingInfo struct {
					TrackingNumber string `json:"trackingNumber"`
				} `json:"trackingInfo"`
				ShipmentDetails struct {
					PossibleDeliveryWindows []struct {
						Description string `json:"description"`
					} `json:"possibleDeliveryWindows"`
				} `json:"shipmentDetails"`
				DateAndTimes []struct {
					Type     string `json:"type"`
					DateTime string `json:"dateTime"`
				} `json:"dateAndTimes"`
				LatestStatusDetail struct {
					StatusByLocale string `json:"statusByLocale"`
					Description    string `json:"description"`
					ScanLocation   struct {
						City          string `json:"city"`
						StateOrProvinceCode string `json:"stateOrProvinceCode"`
						CountryName   string `json:"countryName"`
					} `json:"scanLocation"`
				} `json:"latestStatusDetail"`
				ScanEvents []struct {
					Date         string `json:"date"`
					EventType    string `json:"eventType"`
					EventDescription string `json:"eventDescription"`
					ScanLocation struct {
						City        string `json:"city"`
						CountryName string `json:"countryName"`
					} `json:"scanLocation"`
				} `json:"scanEvents"`
			} `json:"trackResults"`
		} `json:"completeTrackResults"`
	} `json:"output"`
}

func formatFedExResponse(trackingNumber string, body []byte) string {
	var resp fedexTrackResponse
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Output.CompleteTrackResults) == 0 {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "", "  ") == nil {
			return fmt.Sprintf("FedEx tracking for %s:\n%s", trackingNumber, pretty.String())
		}
		return fmt.Sprintf("FedEx tracking for %s:\n%s", trackingNumber, string(body))
	}

	results := resp.Output.CompleteTrackResults[0].TrackResults
	if len(results) == 0 {
		return fmt.Sprintf("FedEx: no tracking results found for %s", trackingNumber)
	}

	r := results[0]
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# FedEx Shipment: %s\n\n", trackingNumber))

	status := r.LatestStatusDetail.StatusByLocale
	if status == "" {
		status = r.LatestStatusDetail.Description
	}
	if status != "" {
		sb.WriteString(fmt.Sprintf("**Status**: %s\n", status))
	}

	loc := r.LatestStatusDetail.ScanLocation
	if loc.City != "" {
		sb.WriteString(fmt.Sprintf("**Current Location**: %s, %s %s\n", loc.City, loc.StateOrProvinceCode, loc.CountryName))
	}

	for _, dt := range r.DateAndTimes {
		if dt.Type == "ESTIMATED_DELIVERY" && dt.DateTime != "" {
			sb.WriteString(fmt.Sprintf("**Estimated Delivery**: %s\n", dt.DateTime))
			break
		}
	}

	if len(r.ScanEvents) > 0 {
		sb.WriteString("\n## Tracking History\n")
		limit := 5
		if len(r.ScanEvents) < limit {
			limit = len(r.ScanEvents)
		}
		for i := 0; i < limit; i++ {
			ev := r.ScanEvents[i]
			evLoc := ev.ScanLocation.City
			if ev.ScanLocation.CountryName != "" {
				if evLoc != "" {
					evLoc += ", " + ev.ScanLocation.CountryName
				} else {
					evLoc = ev.ScanLocation.CountryName
				}
			}
			if evLoc != "" {
				sb.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", ev.Date, ev.EventDescription, evLoc))
			} else {
				sb.WriteString(fmt.Sprintf("- **%s** — %s\n", ev.Date, ev.EventDescription))
			}
		}
	}
	return sb.String()
}

// --- SF Express ---
//
// SF Express Open Platform: https://open.sf-express.com/
// Key format: app_id:app_key (colon-separated).
// Endpoint: POST https://bsp-oisp.sf-express.com/bsp-oisp/sf-express/route.do
// Uses XML-over-HTTP with MD5 signature: base64(md5(xmlBody + appKey)).

const sfExpressTrackURL = "https://bsp-oisp.sf-express.com/bsp-oisp/sf-express/route.do"

func (t *TrackShipmentTool) trackSFExpress(ctx context.Context, trackingNumber string) *Result {
	rawKey := t.getKey("sf_express")
	if rawKey == "" {
		return TextResult("No API key configured for SF Express. Configure it in Settings → Provider Keys (category: tracking, provider: sf_express). Key format: app_id:app_key")
	}
	parts := strings.SplitN(rawKey, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ErrorResult("sf_express: invalid key format. Expected app_id:app_key (colon-separated).")
	}
	appID, appKey := parts[0], parts[1]

	// Build XML request body
	xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><Request service="RouteService" lang="en"><Head>%s</Head><Body><RouteRequest trackingType="1" methodType="1"><TrackingNumber>%s</TrackingNumber></RouteRequest></Body></Request>`, appID, trackingNumber)

	// Signature: base64(md5(xmlBody + appKey))
	sigInput := xmlBody + appKey
	hash := md5.Sum([]byte(sigInput))
	verifyCode := base64.StdEncoding.EncodeToString(hash[:])

	formData := url.Values{}
	formData.Set("xml", xmlBody)
	formData.Set("verifyCode", verifyCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sfExpressTrackURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return ErrorResult(fmt.Sprintf("sf_express: failed to build request: %v", err))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.http.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("sf_express: request failed: %v", err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrorResult("sf_express: credentials rejected. Verify the app_id and app_key in Settings → Provider Keys.")
	}
	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("sf_express: unexpected HTTP %d: %s", resp.StatusCode, truncateErr(string(body), 300)))
	}

	return TextResult(formatSFExpressResponse(trackingNumber, body))
}

func formatSFExpressResponse(trackingNumber string, body []byte) string {
	// SF Express returns XML. Parse key fields with simple string scan for robustness.
	rawXML := string(body)

	// Check for error response
	if strings.Contains(rawXML, `<ERROR>`) || strings.Contains(rawXML, `apiResultCode="999`) {
		return fmt.Sprintf("SF Express: could not retrieve tracking for %s. The waybill number may be incorrect or the shipment is not yet registered.", trackingNumber)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# SF Express Shipment: %s\n\n", trackingNumber))

	// Extract route stop entries (<RouteResponse> contains <Route> elements)
	routes := extractXMLRepeating(rawXML, "Route")
	if len(routes) == 0 {
		sb.WriteString("No tracking events found. The shipment may not yet be in transit.\n")
		sb.WriteString(fmt.Sprintf("\nRaw response:\n%s\n", truncateErr(rawXML, 800)))
		return sb.String()
	}

	// Most recent event first (SF Express returns chronological, newest at end)
	latest := routes[len(routes)-1]
	if status := xmlAttr(latest, "remark"); status != "" {
		sb.WriteString(fmt.Sprintf("**Status**: %s\n", status))
	}
	if loc := xmlAttr(latest, "acceptAddress"); loc != "" {
		sb.WriteString(fmt.Sprintf("**Current Location**: %s\n", loc))
	}
	if ts := xmlAttr(latest, "acceptTime"); ts != "" {
		sb.WriteString(fmt.Sprintf("**Last Updated**: %s\n", ts))
	}

	sb.WriteString("\n## Tracking History\n")
	limit := 5
	if len(routes) < limit {
		limit = len(routes)
	}
	// Print newest first
	for i := len(routes) - 1; i >= len(routes)-limit; i-- {
		r := routes[i]
		ts := xmlAttr(r, "acceptTime")
		desc := xmlAttr(r, "remark")
		loc := xmlAttr(r, "acceptAddress")
		if loc != "" {
			sb.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", ts, desc, loc))
		} else {
			sb.WriteString(fmt.Sprintf("- **%s** — %s\n", ts, desc))
		}
	}
	return sb.String()
}

// --- YTO Express ---
//
// YTO Express Open API: https://open.yto56.com.cn/
// Key format: app_id:app_key (colon-separated).
// Endpoint: POST https://open.yto56.com.cn/open/api
// JSON body signed with MD5(paramJSON + appKey).

const ytoTrackURL = "https://open.yto56.com.cn/open/api"

func (t *TrackShipmentTool) trackYTO(ctx context.Context, trackingNumber string) *Result {
	rawKey := t.getKey("yto")
	if rawKey == "" {
		return TextResult("No API key configured for YTO Express. Configure it in Settings → Provider Keys (category: tracking, provider: yto). Key format: app_id:app_key")
	}
	parts := strings.SplitN(rawKey, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ErrorResult("yto: invalid key format. Expected app_id:app_key (colon-separated).")
	}
	appID, appKey := parts[0], parts[1]

	paramData := map[string]any{
		"waybillNo": trackingNumber,
	}
	paramBytes, _ := json.Marshal(paramData)
	paramStr := string(paramBytes)

	// Signature: md5(paramJSON + appKey), hex lowercase
	sigInput := paramStr + appKey
	hash := md5.Sum([]byte(sigInput))
	sign := fmt.Sprintf("%x", hash)

	reqBody := map[string]any{
		"param":   paramStr,
		"sign":    sign,
		"partner": appID,
		"method":  "yto.elec.member.order.track",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ytoTrackURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ErrorResult(fmt.Sprintf("yto: failed to build request: %v", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("yto: request failed: %v", err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("yto: unexpected HTTP %d: %s", resp.StatusCode, truncateErr(string(body), 300)))
	}

	return TextResult(formatYTOResponse(trackingNumber, body))
}

type ytoTrackResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Result  struct {
		WaybillNo  string `json:"waybillNo"`
		LastStatus string `json:"lastStatus"`
		Tracks     []struct {
			Time    string `json:"time"`
			OpName  string `json:"opName"`
			OpCode  string `json:"opCode"`
			Scanner string `json:"scanner"`
			Info    string `json:"info"`
		} `json:"tracks"`
	} `json:"result"`
}

func formatYTOResponse(trackingNumber string, body []byte) string {
	var resp ytoTrackResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "", "  ") == nil {
			return fmt.Sprintf("YTO Express tracking for %s:\n%s", trackingNumber, pretty.String())
		}
		return fmt.Sprintf("YTO Express tracking for %s:\n%s", trackingNumber, string(body))
	}
	if resp.Code != "0" && resp.Code != "200" && resp.Code != "" {
		return fmt.Sprintf("YTO Express error for %s: %s (code: %s)", trackingNumber, resp.Message, resp.Code)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# YTO Express Shipment: %s\n\n", trackingNumber))
	if resp.Result.LastStatus != "" {
		sb.WriteString(fmt.Sprintf("**Status**: %s\n", resp.Result.LastStatus))
	}

	if len(resp.Result.Tracks) > 0 {
		sb.WriteString("\n## Tracking History\n")
		limit := 5
		if len(resp.Result.Tracks) < limit {
			limit = len(resp.Result.Tracks)
		}
		for i := 0; i < limit; i++ {
			tr := resp.Result.Tracks[i]
			loc := tr.Scanner
			if loc != "" {
				sb.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", tr.Time, tr.Info, loc))
			} else {
				sb.WriteString(fmt.Sprintf("- **%s** — %s\n", tr.Time, tr.Info))
			}
		}
	}
	return sb.String()
}

// --- STO Express ---
//
// STO Express Open API: https://openapi.sto.cn/
// Key format: app_id:app_key (colon-separated).
// Endpoint: POST https://openapi.sto.cn/open/api/route
// Signature: md5(appID + body + appKey), hex uppercase.

const stoTrackURL = "https://openapi.sto.cn/open/api/route"

func (t *TrackShipmentTool) trackSTO(ctx context.Context, trackingNumber string) *Result {
	rawKey := t.getKey("sto")
	if rawKey == "" {
		return TextResult("No API key configured for STO Express. Configure it in Settings → Provider Keys (category: tracking, provider: sto). Key format: app_id:app_key")
	}
	parts := strings.SplitN(rawKey, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ErrorResult("sto: invalid key format. Expected app_id:app_key (colon-separated).")
	}
	appID, appKey := parts[0], parts[1]

	reqData := map[string]any{
		"waybillNo": trackingNumber,
		"timeRange": 30,
	}
	dataBytes, _ := json.Marshal(reqData)
	dataStr := string(dataBytes)

	// Signature: MD5(appID + dataStr + appKey), hex uppercase
	sigInput := appID + dataStr + appKey
	hash := md5.Sum([]byte(sigInput))
	sign := strings.ToUpper(fmt.Sprintf("%x", hash))

	bodyData := map[string]any{
		"appId": appID,
		"data":  dataStr,
		"sign":  sign,
	}
	bodyBytes, _ := json.Marshal(bodyData)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stoTrackURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ErrorResult(fmt.Sprintf("sto: failed to build request: %v", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("sto: request failed: %v", err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("sto: unexpected HTTP %d: %s", resp.StatusCode, truncateErr(string(body), 300)))
	}

	return TextResult(formatSTOResponse(trackingNumber, body))
}

type stoTrackResponse struct {
	Success bool   `json:"success"`
	ErrCode string `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
	Data    []struct {
		WaybillNo  string `json:"waybillNo"`
		Status     string `json:"status"`
		StatusDesc string `json:"statusDesc"`
		Routes     []struct {
			OpTime   string `json:"opTime"`
			OpCode   string `json:"opCode"`
			Remark   string `json:"remark"`
			OpCenter string `json:"opCenter"`
		} `json:"routes"`
	} `json:"data"`
}

func formatSTOResponse(trackingNumber string, body []byte) string {
	var resp stoTrackResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "", "  ") == nil {
			return fmt.Sprintf("STO Express tracking for %s:\n%s", trackingNumber, pretty.String())
		}
		return fmt.Sprintf("STO Express tracking for %s:\n%s", trackingNumber, string(body))
	}
	if !resp.Success {
		return fmt.Sprintf("STO Express error for %s: %s (code: %s)", trackingNumber, resp.ErrMsg, resp.ErrCode)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# STO Express Shipment: %s\n\n", trackingNumber))

	if len(resp.Data) == 0 {
		sb.WriteString("No tracking data found.")
		return sb.String()
	}

	d := resp.Data[0]
	statusDesc := d.StatusDesc
	if statusDesc == "" {
		statusDesc = d.Status
	}
	if statusDesc != "" {
		sb.WriteString(fmt.Sprintf("**Status**: %s\n", statusDesc))
	}

	if len(d.Routes) > 0 {
		sb.WriteString("\n## Tracking History\n")
		limit := 5
		if len(d.Routes) < limit {
			limit = len(d.Routes)
		}
		for i := 0; i < limit; i++ {
			r := d.Routes[i]
			if r.OpCenter != "" {
				sb.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", r.OpTime, r.Remark, r.OpCenter))
			} else {
				sb.WriteString(fmt.Sprintf("- **%s** — %s\n", r.OpTime, r.Remark))
			}
		}
	}
	return sb.String()
}

// --- Best Express ---
//
// Best Express Open API: https://open.800best.com/
// Key format: app_id:app_secret (colon-separated).
// Endpoint: POST https://open.800best.com/bestapi/waybillTrace.do
// Signature: MD5(appID + trackingNumber + appSecret), hex uppercase.

const bestTrackURL = "https://open.800best.com/bestapi/waybillTrace.do"

func (t *TrackShipmentTool) trackBest(ctx context.Context, trackingNumber string) *Result {
	rawKey := t.getKey("best")
	if rawKey == "" {
		return TextResult("No API key configured for Best Express. Configure it in Settings → Provider Keys (category: tracking, provider: best). Key format: app_id:app_secret")
	}
	parts := strings.SplitN(rawKey, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ErrorResult("best: invalid key format. Expected app_id:app_secret (colon-separated).")
	}
	appID, appSecret := parts[0], parts[1]

	// Signature: MD5(appID + trackingNumber + appSecret), hex uppercase
	sigInput := appID + trackingNumber + appSecret
	hash := md5.Sum([]byte(sigInput))
	sign := strings.ToUpper(fmt.Sprintf("%x", hash))

	formData := url.Values{}
	formData.Set("partner_id", appID)
	formData.Set("waybill_no", trackingNumber)
	formData.Set("sign", sign)
	formData.Set("lang", "en")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bestTrackURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return ErrorResult(fmt.Sprintf("best: failed to build request: %v", err))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("best: request failed: %v", err))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("best: unexpected HTTP %d: %s", resp.StatusCode, truncateErr(string(body), 300)))
	}

	return TextResult(formatBestResponse(trackingNumber, body))
}

type bestTrackResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		WaybillNo string `json:"waybillNo"`
		Status    string `json:"status"`
		Traces    []struct {
			OpTime   string `json:"opTime"`
			OpDesc   string `json:"opDesc"`
			OpCity   string `json:"opCity"`
			OrgName  string `json:"orgName"`
		} `json:"traces"`
	} `json:"data"`
}

func formatBestResponse(trackingNumber string, body []byte) string {
	var resp bestTrackResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "", "  ") == nil {
			return fmt.Sprintf("Best Express tracking for %s:\n%s", trackingNumber, pretty.String())
		}
		return fmt.Sprintf("Best Express tracking for %s:\n%s", trackingNumber, string(body))
	}
	if resp.Code != "0" && resp.Code != "200" && resp.Code != "OK" && resp.Code != "" {
		return fmt.Sprintf("Best Express error for %s: %s (code: %s)", trackingNumber, resp.Message, resp.Code)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Best Express Shipment: %s\n\n", trackingNumber))
	if resp.Data.Status != "" {
		sb.WriteString(fmt.Sprintf("**Status**: %s\n", resp.Data.Status))
	}

	if len(resp.Data.Traces) > 0 {
		sb.WriteString("\n## Tracking History\n")
		limit := 5
		if len(resp.Data.Traces) < limit {
			limit = len(resp.Data.Traces)
		}
		for i := 0; i < limit; i++ {
			tr := resp.Data.Traces[i]
			loc := tr.OpCity
			if tr.OrgName != "" {
				if loc != "" {
					loc = tr.OrgName + ", " + loc
				} else {
					loc = tr.OrgName
				}
			}
			if loc != "" {
				sb.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", tr.OpTime, tr.OpDesc, loc))
			} else {
				sb.WriteString(fmt.Sprintf("- **%s** — %s\n", tr.OpTime, tr.OpDesc))
			}
		}
	}
	return sb.String()
}

// extractXMLRepeating extracts all occurrences of <tagName .../> or <tagName ...>...</tagName>
// from an XML string. Used for SF Express XML response parsing without importing encoding/xml.
func extractXMLRepeating(xmlStr, tagName string) []string {
	var results []string
	openTag := "<" + tagName
	closeTag := "</" + tagName + ">"
	selfClose := "/>"
	pos := 0
	for {
		start := strings.Index(xmlStr[pos:], openTag)
		if start < 0 {
			break
		}
		start += pos
		// Find end of this element
		endSelf := strings.Index(xmlStr[start:], selfClose)
		endClose := strings.Index(xmlStr[start:], closeTag)
		var end int
		if endSelf >= 0 && (endClose < 0 || endSelf < endClose) {
			end = start + endSelf + len(selfClose)
		} else if endClose >= 0 {
			end = start + endClose + len(closeTag)
		} else {
			break
		}
		results = append(results, xmlStr[start:end])
		pos = end
	}
	return results
}

// xmlAttr extracts an attribute value from a single XML element string.
// e.g. xmlAttr(`<Route remark="Delivered" acceptTime="2024-01-01"/>`, "remark") → "Delivered"
func xmlAttr(element, attr string) string {
	needle := attr + `="`
	idx := strings.Index(element, needle)
	if idx < 0 {
		return ""
	}
	rest := element[idx+len(needle):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}
