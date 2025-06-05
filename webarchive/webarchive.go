package webarchive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"postmark-inbound/htmlutil"
	"strconv"
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
			Timeout: 60 * time.Second,
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
	apiURL := fmt.Sprintf("https://web.archive.org/cdx/search/cdx?url=%s&fl=timestamp,mimetype,statuscode,digest,length&output=json&fastLatest=true&limit=-10", encodedURL)

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
		if len(row) < 5 {
			continue
		}
		if row[2] == "-" {
			continue
		}
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

	return snapshots, nil
}

func formatTimestamp(ts time.Time) string {
	return ts.Format("20060102150405")
}

func (c *Client) LoadSnapshot(originalURL string, timestamp time.Time) (string, error) {
	snapshotURL := fmt.Sprintf("https://web.archive.org/web/%s/%s", formatTimestamp(timestamp), originalURL)

	log.Printf("Snapshot url %q", snapshotURL)

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

	textContent, err := htmlutil.ExtractText(newWaybackToolbarStripper(resp.Body))
	if err != nil {
		return "", fmt.Errorf("failed to extract text from HTML: %w", err)
	}

	return textContent, nil
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

type waybackToolbarStripper struct {
	reader      io.Reader
	buffer      []byte
	bufferPos   int
	inToolbar   bool
	startMarker []byte
	endMarker   []byte
	matchPos    int
	endMatchPos int
}

func newWaybackToolbarStripper(reader io.Reader) *waybackToolbarStripper {
	return &waybackToolbarStripper{
		reader:      reader,
		buffer:      make([]byte, 0, 8192),
		startMarker: []byte("<!-- BEGIN WAYBACK TOOLBAR INSERT -->"),
		endMarker:   []byte("<!-- END WAYBACK TOOLBAR INSERT -->"),
	}
}

func (w *waybackToolbarStripper) Read(p []byte) (n int, err error) {
	for n < len(p) {
		if w.bufferPos >= len(w.buffer) {
			readBuf := make([]byte, 4096)
			readN, readErr := w.reader.Read(readBuf)
			if readN > 0 {
				w.buffer = append(w.buffer, readBuf[:readN]...)
			}
			if readErr != nil {
				if readErr == io.EOF && w.bufferPos >= len(w.buffer) {
					return n, io.EOF
				} else if readErr != io.EOF {
					return n, readErr
				}
			}
			if w.bufferPos >= len(w.buffer) && readErr == io.EOF {
				return n, io.EOF
			}
		}

		currentByte := w.buffer[w.bufferPos]
		w.bufferPos++

		if w.inToolbar {
			if w.endMatchPos < len(w.endMarker) && currentByte == w.endMarker[w.endMatchPos] {
				w.endMatchPos++
				if w.endMatchPos == len(w.endMarker) {
					w.inToolbar = false
					w.endMatchPos = 0
				}
			} else {
				w.endMatchPos = 0
				if currentByte == w.endMarker[0] {
					w.endMatchPos = 1
				}
			}
		} else {
			if w.matchPos < len(w.startMarker) && currentByte == w.startMarker[w.matchPos] {
				w.matchPos++
				if w.matchPos == len(w.startMarker) {
					w.inToolbar = true
					w.matchPos = 0
				}
			} else {
				if w.matchPos > 0 {
					copy(p[n:], w.startMarker[:w.matchPos])
					n += w.matchPos
					w.matchPos = 0
					if n >= len(p) {
						w.bufferPos--
						return n, nil
					}
				}
				if currentByte == w.startMarker[0] {
					w.matchPos = 1
				} else {
					p[n] = currentByte
					n++
				}
			}
		}
	}
	return n, nil
}
