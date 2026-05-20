// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
package gateway

import (
	"bufio"
	"compress/gzip"
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	cfLogBucket = "qorven-install-logs"
	cfLogPrefix = "cf-logs/"
)

// InstallEvent is one parsed install ping from the /t query string captured
// in the CloudFront access log's cs-uri-query field.
type InstallEvent struct {
	Date     string `json:"date"`
	Time     string `json:"time"`
	IP       string `json:"ip"`
	EdgeLoc  string `json:"edge_location"`
	Version  string `json:"version"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Kernel   string `json:"kernel"`
	Distro   string `json:"distro"`
	Cores    string `json:"cores"`
	MemGB    string `json:"mem_gb"`
	HostHash string `json:"host_hash"`
	Cloud    string `json:"cloud"`
}

// cfLogFields maps the positional index of each CloudFront W3C log field.
// Fields: date time x-edge-location sc-bytes c-ip cs-method cs(Host)
//
//	cs-uri-stem sc-status ... cs-uri-query ...
const (
	cfFieldDate      = 0
	cfFieldTime      = 1
	cfFieldEdgeLoc   = 2
	cfFieldIP        = 4
	cfFieldMethod    = 5
	cfFieldURIStem   = 7
	cfFieldStatus    = 8
	cfFieldURIQuery  = 11
)

func (gw *Gateway) handleInstallAnalytics(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	events, err := parseCFInstallLogs(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	writeJSON(w, http.StatusOK, buildInstallStats(events))
}

// parseCFInstallLogs reads all CF log files from S3 and returns every request
// that hit /t (the install telemetry pixel).
func parseCFInstallLogs(ctx context.Context) ([]InstallEvent, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg)

	var events []InstallEvent

	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(cfLogBucket),
		Prefix: aws.String(cfLogPrefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			evs, err := parseCFLogObject(ctx, client, aws.ToString(obj.Key))
			if err != nil {
				continue // skip unreadable files
			}
			events = append(events, evs...)
		}
	}
	return events, nil
}

func parseCFLogObject(ctx context.Context, client *s3.Client, key string) ([]InstallEvent, error) {
	resp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(cfLogBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	var events []InstallEvent
	scanner := bufio.NewScanner(gr)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}
		if fields[cfFieldURIStem] != "/t" || fields[cfFieldStatus] != "200" {
			continue
		}
		if fields[cfFieldMethod] != "GET" {
			continue
		}

		ev := InstallEvent{
			Date:    fields[cfFieldDate],
			Time:    fields[cfFieldTime],
			IP:      fields[cfFieldIP],
			EdgeLoc: edgeLocCity(fields[cfFieldEdgeLoc]),
		}

		// Parse query string: v=..&os=..&arch=..&kernel=..&distro=..&cores=..&mem=..&host=..&cloud=..
		if q := fields[cfFieldURIQuery]; q != "-" && q != "" {
			vals, _ := url.ParseQuery(q)
			ev.Version  = vals.Get("v")
			ev.OS       = vals.Get("os")
			ev.Arch     = vals.Get("arch")
			ev.Kernel   = vals.Get("kernel")
			ev.Distro   = vals.Get("distro")
			ev.Cores    = vals.Get("cores")
			ev.MemGB    = vals.Get("mem")
			ev.HostHash = vals.Get("host")
			ev.Cloud    = vals.Get("cloud")
		}

		events = append(events, ev)
	}
	return events, scanner.Err()
}

type InstallStats struct {
	Total     int                    `json:"total"`
	Events    []InstallEvent         `json:"events"`
	ByVersion map[string]int         `json:"by_version"`
	ByOS      map[string]int         `json:"by_os"`
	ByArch    map[string]int         `json:"by_arch"`
	ByDistro  map[string]int         `json:"by_distro"`
	ByCloud   map[string]int         `json:"by_cloud"`
	ByDate    map[string]int         `json:"by_date"`
	ByEdge    map[string]int         `json:"by_edge_location"`
	UniqueIPs int                    `json:"unique_ips"`
}

func buildInstallStats(events []InstallEvent) InstallStats {
	// sort newest first
	sort.Slice(events, func(i, j int) bool {
		di, dj := events[i].Date+" "+events[i].Time, events[j].Date+" "+events[j].Time
		return di > dj
	})

	stats := InstallStats{
		Total:     len(events),
		Events:    events,
		ByVersion: map[string]int{},
		ByOS:      map[string]int{},
		ByArch:    map[string]int{},
		ByDistro:  map[string]int{},
		ByCloud:   map[string]int{},
		ByDate:    map[string]int{},
		ByEdge:    map[string]int{},
	}
	ips := map[string]struct{}{}

	for _, e := range events {
		if e.Version != "" { stats.ByVersion[e.Version]++ }
		if e.OS      != "" { stats.ByOS[e.OS]++ }
		if e.Arch    != "" { stats.ByArch[e.Arch]++ }
		if e.Distro  != "" { stats.ByDistro[e.Distro]++ }
		if e.Cloud   != "" { stats.ByCloud[e.Cloud]++ }
		if e.Date    != "" { stats.ByDate[e.Date]++ }
		if e.EdgeLoc != "" { stats.ByEdge[e.EdgeLoc]++ }
		if e.IP      != "" { ips[e.IP] = struct{}{} }
	}
	stats.UniqueIPs = len(ips)
	return stats
}

// edgeLocCity converts a CloudFront edge POP code like "BOM54-P2" to a
// human-readable city name. Unknown codes are returned as-is.
func edgeLocCity(code string) string {
	prefix := strings.ToUpper(code)
	if len(prefix) >= 3 {
		prefix = prefix[:3]
	}
	cities := map[string]string{
		"BOM": "Mumbai", "SIN": "Singapore", "NRT": "Tokyo", "ICN": "Seoul",
		"SYD": "Sydney", "HKG": "Hong Kong", "MNL": "Manila", "CGK": "Jakarta",
		"BKK": "Bangkok", "DEL": "Delhi", "MAA": "Chennai", "HYD": "Hyderabad",
		"CCU": "Kolkata", "CMB": "Colombo", "DAC": "Dhaka",
		"IAD": "Ashburn (US East)", "JFK": "New York", "ORD": "Chicago",
		"LAX": "Los Angeles", "SFO": "San Francisco", "SEA": "Seattle",
		"DFW": "Dallas", "ATL": "Atlanta", "MIA": "Miami", "BOS": "Boston",
		"YYZ": "Toronto", "YVR": "Vancouver", "GRU": "São Paulo", "BOG": "Bogotá",
		"LHR": "London", "CDG": "Paris", "FRA": "Frankfurt", "AMS": "Amsterdam",
		"ARN": "Stockholm", "CPH": "Copenhagen", "MXP": "Milan", "MAD": "Madrid",
		"WAW": "Warsaw", "PRG": "Prague", "VIE": "Vienna", "ZRH": "Zurich",
		"DXB": "Dubai", "BAH": "Bahrain", "CAI": "Cairo", "JNB": "Johannesburg",
		"NBO": "Nairobi", "LOS": "Lagos",
	}
	if city, ok := cities[prefix]; ok {
		return city
	}
	return code
}
