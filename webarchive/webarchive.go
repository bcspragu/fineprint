package webarchive

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
	URLKey     string `json:"urlkey"`
	Timestamp  string `json:"timestamp"`
	Original   string `json:"original"`
	MimeType   string `json:"mimetype"`
	StatusCode string `json:"statuscode"`
	Digest     string `json:"digest"`
	Length     string `json:"length"`
}

func (c *Client) GetSnapshots(targetURL string) ([]Snapshot, error) {
	encodedURL := url.QueryEscape(targetURL)
	apiURL := fmt.Sprintf("http://web.archive.org/cdx/search/cdx?url=%s&output=json", encodedURL)

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

	snapshots := make([]Snapshot, 0, len(rawData)-1)
	for i := 1; i < len(rawData); i++ {
		row := rawData[i]
		if len(row) >= 7 {
			snapshot := Snapshot{
				URLKey:     row[0],
				Timestamp:  row[1],
				Original:   row[2],
				MimeType:   row[3],
				StatusCode: row[4],
				Digest:     row[5],
				Length:     row[6],
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
