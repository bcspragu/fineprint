package tosdr

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type SearchService struct {
	ID int `json:"id"`

	IsComprehensivelyReviewed bool `json:"is_comprehensively_reviewed"`

	URLs      []string `json:"urls"`
	Name      string   `json:"name"`
	UpdatedAt string   `json:"updated_at"`
	CreatedAt string   `json:"created_at"`
	Slug      string   `json:"slug"`
	Rating    string   `json:"rating"` // Looks like A through F?
}

type Service struct {
	ID int `json:"id"`

	IsComprehensivelyReviewed bool `json:"is_comprehensively_reviewed"`

	Name      string     `json:"name"`
	UpdatedAt string     `json:"updated_at"`
	CreatedAt string     `json:"created_at"`
	Slug      string     `json:"slug"`
	Rating    string     `json:"rating"`
	URLs      []string   `json:"urls"`
	Image     string     `json:"image"`
	Documents []Document `json:"documents"`
	Points    []Point    `json:"points"`
}

type Document struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Point struct {
	ID    int    `json:"id"`
	Title string `json:"title"`

	Source   string `json:"source"`
	Status   string `json:"status"`
	Analysis string `json:"analysis"`
	Case     *Case  `json:"case"`

	DocumentID int `json:"document_id"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Case struct {
	ID             int    `json:"id"`
	Weight         int    `json:"weight"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	UpdatedAt      string `json:"updated_at"`
	CreatedAt      string `json:"created_at"`
	TopicID        int    `json:"topic_id"`
	Classification string `json:"classification"` // "good", "blocker", "neutral", "bad"
}

type SearchResponse struct {
	Services []SearchService `json:"services"`
}

func SearchServices(companyName string) (*SearchResponse, error) {
	if companyName == "" {
		return nil, fmt.Errorf("company name is required")
	}

	baseURL := "https://api.tosdr.org/search/v5/"
	params := url.Values{}
	params.Add("query", companyName)

	fullURL := baseURL + "?" + params.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close body on ToS;DR API request: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ToS;DR API returned status: %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("error decoding ToS;DR response: %v", err)
	}

	return &searchResp, nil
}

func GetService(serviceID int) (*Service, error) {
	baseURL := fmt.Sprintf("https://api.tosdr.org/service/v3/?id=%d", serviceID)

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close body on ToS;DR API request: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ToS;DR API returned status: %d", resp.StatusCode)
	}

	var service Service
	if err := json.NewDecoder(resp.Body).Decode(&service); err != nil {
		return nil, fmt.Errorf("error decoding ToS;DR response: %v", err)
	}

	return &service, nil
}

func GetDocument(documentID int) (*Document, error) {
	baseURL := fmt.Sprintf("https://api.tosdr.org/document/v1?id=%d", documentID)

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close body on ToS;DR API request: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ToS;DR API returned status: %d", resp.StatusCode)
	}

	var document Document
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		return nil, fmt.Errorf("error decoding ToS;DR response: %v", err)
	}

	return &document, nil
}

func FindBestServiceMatch(searchResults *SearchResponse, domain string) *SearchService {
	if searchResults == nil || len(searchResults.Services) == 0 {
		return nil
	}

	domain = strings.ToLower(strings.TrimSpace(domain))

	for _, service := range searchResults.Services {
		for _, serviceURL := range service.URLs {
			serviceURL = strings.ToLower(strings.TrimSpace(serviceURL))

			if serviceURL == domain {
				return &service
			}

			if strings.Contains(serviceURL, domain) || strings.Contains(domain, serviceURL) {
				return &service
			}
		}
	}

	if len(searchResults.Services) > 0 {
		return &searchResults.Services[0]
	}

	return nil
}
