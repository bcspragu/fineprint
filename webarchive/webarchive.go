package webarchive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	AccessKey  string
	SecretKey  string
	HTTPClient *http.Client
}

func NewClient(accessKey, secretKey string) *Client {
	return &Client{
		AccessKey: accessKey,
		SecretKey: secretKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type Snapshot struct {
	Timestamp  time.Time
	MimeType   string
	StatusCode int
	Digest     string
	Length     int
}

func (c *Client) GetSnapshots(targetURL string) ([]Snapshot, error) {
	encodedURL := url.QueryEscape(targetURL)
	apiURL := fmt.Sprintf("http://web.archive.org/cdx/search/cdx?url=%s&fl=timestamp,mimetype,statuscode,digest,length&output=json", encodedURL)

	resp, err := c.HTTPClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch snapshots: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close body on Web Archive API request: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var rawData [][]string
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if len(rawData) == 0 {
		return []Snapshot{}, nil
	}

	var rErr error
	parse := func(inp string) int {
		if rErr != nil {
			return 0
		}
		v, err := strconv.Atoi(inp)
		if err != nil {
			rErr = err
		}
		return v
	}

	snapshots := make([]Snapshot, 0, len(rawData)-1)
	for i := 1; i < len(rawData); i++ {
		row := rawData[i]
		if len(row) >= 5 {
			ts, err := parseTimestamp(row[0])
			if err != nil {
				return nil, fmt.Errorf("failed to parse IA timestamp %q: %w", row[0], err)
			}
			snapshot := Snapshot{
				Timestamp:  ts,
				MimeType:   row[1],
				StatusCode: parse(row[2]),
				Digest:     row[3],
				Length:     parse(row[4]),
			}
			if rErr != nil {
				return nil, fmt.Errorf("failed to parse date string: %w", rErr)
			}
			snapshots = append(snapshots, snapshot)
		}
	}

	return snapshots, nil
}

func (c *Client) LoadSnapshot(originalURL, timestamp string) (string, error) {
	snapshotURL := fmt.Sprintf("http://web.archive.org/web/%s/%s", timestamp, originalURL)

	resp, err := c.HTTPClient.Get(snapshotURL)
	if err != nil {
		return "", fmt.Errorf("failed to load snapshot: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close body on Web Archive API request: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("snapshot request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read snapshot content: %w", err)
	}

	content := string(body)

	content = c.removeWaybackToolbar(content)

	return content, nil
}

func (c *Client) removeWaybackToolbar(content string) string {
	startMarker := "<!-- BEGIN WAYBACK TOOLBAR INSERT -->"
	endMarker := "<!-- END WAYBACK TOOLBAR INSERT -->"

	start := strings.Index(content, startMarker)
	if start == -1 {
		return content
	}

	end := strings.Index(content, endMarker)
	if end == -1 {
		return content
	}

	return content[:start] + content[end+len(endMarker):]
}

func parseTimestamp(ts string) (time.Time, error) {
	if len(ts) < 14 {
		return time.Time{}, errors.New("ts string was too short")
	}

	var rErr error
	parse := func(inp string) int {
		if rErr != nil {
			return 0
		}
		v, err := strconv.Atoi(inp)
		if err != nil {
			rErr = err
		}
		return v
	}

	yr := parse(ts[:4])
	month := time.Month(parse(ts[4:6]))
	day := parse(ts[6:8])
	hr := parse(ts[8:10])
	min := parse(ts[10:12])
	sec := parse(ts[12:14])

	if rErr != nil {
		return time.Time{}, fmt.Errorf("failed to parse date string: %w", rErr)
	}

	return time.Date(yr, month, day, hr, min, sec, 0, time.UTC), nil
}
