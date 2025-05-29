package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/peterbourgon/ff/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"postmark-inbound/claude"
	"postmark-inbound/postmark"
	"postmark-inbound/templates"
	"postmark-inbound/tosdr"
	"postmark-inbound/webarchive"
)

func main() {
	if err := run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// See the Postmark JS example: https://github.com/activecampaign/postmark_webhooks/blob/master/server/main.js#L8
// They authorize based on IP, as opposed to providing signatures we can verify
var authorizedIPs = []string{"3.134.147.250", "50.31.156.6", "50.31.156.77", "18.217.206.57", "127.0.0.1"}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("no args given")
	}

	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	var (
		addr            = fs.String("addr", ":8080", "Address to listen on")
		replyFromEmail  = fs.String("reply-from-email", "", "Email address to send replies from")
		postmarkToken   = fs.String("postmark-server-token", "", "Postmark server token")
		anthropicAPIKey = fs.String("anthropic-api-key", "", "Anthropic API key")

		archiveAccessKey = fs.String("archive-access-key", "", "Internet Archive access key")
		archiveSecretKey = fs.String("archive-secret-key", "", "Internet Archive secret key")
	)

	if err := ff.Parse(fs, args[1:], ff.WithEnvVars()); err != nil {
		log.Fatal("Failed to parse flags:", err)
	}

	webarchiveClient := webarchive.NewClient(*archiveAccessKey, *archiveSecretKey)

	handler := &Handler{
		replyFromEmail:   *replyFromEmail,
		postmarkToken:    *postmarkToken,
		anthropicAPIKey:  *anthropicAPIKey,
		webarchiveClient: webarchiveClient,
	}

	http.HandleFunc("/webhook", handler.handleInboundEmail)

	log.Printf("Server starting on %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		return fmt.Errorf("http.ListenAndServe: %w", err)
	}
	return nil
}

type Handler struct {
	replyFromEmail, postmarkToken, anthropicAPIKey string
	webarchiveClient                               *webarchive.Client
}

func textResponse(w http.ResponseWriter, msg string) {
	if _, err := io.WriteString(w, msg); err != nil {
		log.Printf("failed to write text response: %v", err)
	}
}

func isIPAuthorized(ip string) bool {
	for _, authorizedIP := range authorizedIPs {
		if ip == authorizedIP {
			return true
		}
	}
	return false
}

func (h *Handler) handleInboundEmail(w http.ResponseWriter, r *http.Request) {
	requestIP := r.Header.Get("X-Forwarded-For")
	if !isIPAuthorized(requestIP) {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var email postmark.InboundEmail
	if err := json.NewDecoder(r.Body).Decode(&email); err != nil {
		log.Printf("Error decoding JSON: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	log.Printf("Received email from %s with subject: %s", email.From, email.Subject)

	classification, err := claude.ClassifyPolicyChange(h.anthropicAPIKey, email.Subject, email.TextBody, email.HtmlBody)
	if err != nil {
		log.Printf("Error classifying email: %v", err)
		textResponse(w, "Classification failed")
		return
	}

	log.Printf("Classification result: isPolicyChange=%t, type=%s, company=%s, confidence=%s, policy_url=%s",
		classification.IsPolicyChange, classification.PolicyType, classification.Company, classification.Confidence, classification.PolicyType)

	if !classification.IsPolicyChange {
		log.Printf("Email is not a policy change notification, ignoring")
		textResponse(w, "Email processed - not a policy change")
		return
	}

	// Use heuristics and external APIs to come up with the policy we're looking at.
	// TODO: Currently, we only load ToS;DR results if we didn't get a policy URL
	// from the email/classification, we probably want that part regardless.
	policyResult := h.comeUpWithAPolicyURL(classification)
	if policyResult == nil {
		log.Printf("We couldn't figure out a policy URL, aborting")
		textResponse(w, "Email processed - no policy documents found - probably our fault")
		return
	}

	// If we're here, we can start try loading an older version of the policy for
	// comparison purposes.

	var tosDRResults *tosdr.SearchResponse
	var previousVersion string

	// After thinking about the email format a bit, there's only ~two sections we need to think about:
	//
	// 1. The delta section - Show what's different, only if we found a current + previous version
	// 2. The ToS;DR section - Only if the ToS;DR entry is found
	//
	// And if there's no ToS;DR or past link, but we did find a current one, maybe just an LLM-generated summary in a similar format to ToS;DR

	// With that in mind, what's our protocol?
	//
	// 1. Run classification
	// 2. If we got a policy URL, try to load that directly + via web archive

	// Try to get legal document from first service and load previous version
	if tosDRService != nil {
		previousVersion, err = loadPreviousLegalDocument(tosDRService, &email, h.webarchiveClient, classification.PolicyType)
		if err != nil {
			log.Printf("Error loading previous version: %v", err)
		} else if previousVersion != "" {
			log.Printf("Successfully loaded previous version (%d chars)", len(previousVersion))
		}
	}

	var textSummary string
	if h.replyFromEmail == "" {
		log.Printf("REPLY_FROM_EMAIL not set, skipping email response")
	} else {
		htmlSummary, err := templates.GenerateHTMLEmail(classification, tosDRResults)
		if err != nil {
			log.Printf("Error generating HTML email: %v", err)
		}
		textSummary = generateSummary(classification, tosDRResults)
		subject := fmt.Sprintf("Policy Change Summary: %s", classification.Company)

		messageID := postmark.GetMessageIDFromHeaders(&email)
		err = postmark.SendEmailWithThreading(h.postmarkToken, h.replyFromEmail, email.From, subject, textSummary, htmlSummary, messageID, messageID)
		if err != nil {
			log.Printf("Error sending summary email: %v", err)
		} else {
			log.Printf("Summary email sent to %s", email.From)
		}
	}

	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "Policy change email processed successfully"); err != nil {
		log.Printf("failed to write text response: %v", err)
	}
}

func generateSummary(classification *claude.PolicyClassification, tosDRResults *tosdr.SearchResponse) string {
	var textBody strings.Builder

	title := cases.Title(language.English)

	textBody.WriteString(fmt.Sprintf("Policy Change Summary for %s\n", classification.Company))
	textBody.WriteString("=" + strings.Repeat("=", len("Policy Change Summary for "+classification.Company)) + "\n\n")

	textBody.WriteString(fmt.Sprintf("Policy Type: %s\n", title.String(strings.ReplaceAll(classification.PolicyType, "_", " "))))
	textBody.WriteString(fmt.Sprintf("Classification Confidence: %s\n\n", title.String(classification.Confidence)))

	if tosDRResults != nil && len(tosDRResults.Services) > 0 {
		textBody.WriteString("ToS;DR Analysis Available:\n")

		for _, service := range tosDRResults.Services {
			textBody.WriteString(fmt.Sprintf("• %s (Rating: %s)\n", service.Name, service.Rating))
			if service.IsComprehensive {
				textBody.WriteString("  Comprehensive review available\n")
			}
			textBody.WriteString(fmt.Sprintf("  View at: https://tosdr.org/en/service/%d\n", service.ID))
		}
	} else {
		textBody.WriteString("No existing ToS;DR analysis found for this company.\n")
	}

	textBody.WriteString("\n---\nThis summary was generated automatically by analyzing your forwarded policy change email.")

	return textBody.String()
}

type PolicyLoadResult struct {
	URL          *url.URL
	ResponseBody []byte

	// Only if we loaded things from ToS;DR
	Service *tosdr.Service
}

func (h *Handler) comeUpWithAPolicyURL(classification *claude.PolicyClassification) *PolicyLoadResult {
	type strategy struct {
		name string
		fn   func(*claude.PolicyClassification) string
	}

	var svc *tosdr.Service
	strategies := []strategy{
		{
			name: "use from classification result",
			fn: func(pc *claude.PolicyClassification) string {
				return pc.PolicyURL
			},
		},
		{
			name: "get from ToS;DR",
			fn: func(pc *claude.PolicyClassification) string {
				if strings.TrimSpace(classification.Company) == "" {
					// No company, don't bother
					return ""
				}
				tosDRService, err := h.maybeGetSearchService(classification.Company)
				if err != nil {
					log.Printf("Error getting ToS service: %v", err)
					return ""
				}
				// Can handle a nil `tosDRService`
				doc, err := loadDocument(tosDRService, classification.PolicyType)
				if err != nil {
					log.Printf("Error heuristically getting policy URL: %v", err)
					return ""
				}
				svc = doc.service
				return doc.documentURL
			},
		},
	}

	for _, st := range strategies {
		// For each strategy, try to load the document and see what it do.
		log.Printf("Trying strategy %q", st.name)
		policyURLStr := strings.TrimSpace(st.fn(classification))
		if policyURLStr == "" {
			log.Printf("Strategy %q didn't give us anything", st.name)
			continue
		}

		policyURL, err := url.Parse(policyURLStr)
		if err != nil {
			log.Printf("Strategy %q gave us an invalid url: %v", st.name, err)
			continue
		}

		// Now try to use the policy URL to get stuff
		policyContents, err := getBody(policyURL)
		if err != nil {
			log.Printf("Strategy %q gave us a URL (%q) that we couldn't load: %v", st.name, policyURL.String(), err)
			continue
		}

		// If we're here, I think we're good!
		return &PolicyLoadResult{
			URL:          policyURL,
			ResponseBody: policyContents,
			Service:      svc,
		}
	}

	// Just means we didn't find anything.
	return nil
}

type Document struct {
	documentURL string
	service     *tosdr.Service
}

func loadDocument(service *tosdr.SearchService, policyType string) (*Document, error) {
	if service == nil {
		return nil, nil
	}

	// Get service details to find document URLs
	serviceDetails, err := tosdr.GetService(service.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service details: %w", err)
	}

	var matchFn func(docName string) bool
	switch policyType {
	case "privacy_policy":
		matchFn = func(docName string) bool {
			return strings.Contains(docName, "privacy") || strings.Contains(docName, "data")
		}
	case "terms_of_service":
		matchFn = func(docName string) bool {
			return strings.Contains(docName, "terms") || strings.Contains(docName, "service")
		}
	case "user_agreement":
		matchFn = func(docName string) bool {
			return strings.Contains(docName, "user") || strings.Contains(docName, "agreement")
		}
	case "other":
		// Say yes to the first policy, really just anything we find.
		matchFn = func(_ string) bool { return true }
	}

	// Find the appropriate document URL based on policy type
	var documentURL string
	for _, doc := range serviceDetails.Documents {
		docName := strings.ToLower(doc.Name)
		if matchFn(docName) {
			documentURL = doc.URL
			break
		}
	}

	// If no specific match, try the first document
	if documentURL == "" && len(serviceDetails.Documents) > 0 {
		documentURL = serviceDetails.Documents[0].URL
	}

	return &Document{
		documentURL: documentURL,
		service:     serviceDetails,
	}, nil
}

func (h *Handler) loadPreviousLegalDocument(email *postmark.InboundEmail, documentURL *url.URL) (string, error) {
	// Parse email date
	emailDate, err := time.Parse(time.RFC1123Z, email.Date)
	if err != nil {
		return "", fmt.Errorf("failed to parse email date: %w", err)
	}

	// Get snapshots from Internet Archive
	snapshots, err := h.webarchiveClient.GetSnapshots(documentURL.String())
	if err != nil {
		return "", fmt.Errorf("failed to get snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return "", fmt.Errorf("no snapshots found for URL: %s", documentURL)
	}

	// Find the most recent snapshot that's older than the email date
	var bestSnapshot *webarchive.Snapshot
	for _, snapshot := range snapshots {
		// Parse snapshot timestamp (format: YYYYMMDDhhmmss)
		snapshotTime, err := time.Parse("20060102150405", snapshot.Timestamp)
		if err != nil {
			log.Printf("Failed to parse snapshot timestamp %s: %v", snapshot.Timestamp, err)
			continue
		}

		// Only consider snapshots older than the email date
		if snapshotTime.Before(emailDate) {
			if bestSnapshot == nil || snapshotTime.After(getBestSnapshotTime(bestSnapshot)) {
				bestSnapshot = &snapshot
			}
		}
	}

	if bestSnapshot == nil {
		return "", fmt.Errorf("no snapshots found older than email date %s", emailDate.Format(time.RFC3339))
	}

	log.Printf("Using snapshot from %s for URL %s", bestSnapshot.Timestamp, documentURL)

	// Load the snapshot content
	content, err := webarchiveClient.LoadSnapshot(documentURL, bestSnapshot.Timestamp)
	if err != nil {
		return "", fmt.Errorf("failed to load snapshot: %w", err)
	}

	return content, nil
}

func getBestSnapshotTime(snapshot *webarchive.Snapshot) time.Time {
	t, err := time.Parse("20060102150405", snapshot.Timestamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (h *Handler) maybeGetSearchService(companyName string) (*tosdr.SearchService, error) {
	if companyName == "" {
		// Nothing to go on, return nothing
		return nil, nil
	}

	tosDRResults, err := tosdr.SearchServices(companyName)
	if err != nil {
		return nil, fmt.Errorf("failed to search ToS;DR for %s: %w", companyName, err)
	}

	if len(tosDRResults.Services) == 0 {
		log.Printf("No ToS;DR services found for %s", companyName)
		return nil, nil
	}

	log.Printf("Found %d ToS;DR services for %s:", len(tosDRResults.Services), companyName)
	for _, service := range tosDRResults.Services {
		log.Printf("  - %s (Rating: %s, ID: %d, Comprehensive: %t)",
			service.Name, service.Rating, service.ID, service.IsComprehensive)
	}

	return &tosDRResults.Services[0], nil
}

func getBody(u *url.URL) ([]byte, error) {
	resp, err := http.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("failed to load %q: %w", u.String(), err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// TODO: Probably just extract the <body> element, I can't think of why we'd need anything else.
	return body, nil
}
