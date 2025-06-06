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

	"github.com/hashicorp/go-multierror"
	"github.com/peterbourgon/ff/v3"

	"slices"

	"github.com/bcspragu/fineprint/claude"
	"github.com/bcspragu/fineprint/diff"
	"github.com/bcspragu/fineprint/htmlutil"
	"github.com/bcspragu/fineprint/postmark"
	"github.com/bcspragu/fineprint/ratelimit"
	"github.com/bcspragu/fineprint/templates"
	"github.com/bcspragu/fineprint/tosdr"
	"github.com/bcspragu/fineprint/webarchive"
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
		anthropicAPIKey = fs.String("anthropic-api-key", "", "Anthropic API key")

		postmarkToken           = fs.String("postmark-server-token", "", "Postmark server token")
		postmarkWebhookUsername = fs.String("postmark-webhook-username", "", "The basic auth username we'll receive from Postmark")
		postmarkWebhookPassword = fs.String("postmark-webhook-password", "", "The basic auth password we'll receive from Postmark")

		archiveAccessKey = fs.String("archive-access-key", "", "Internet Archive access key")
		archiveSecretKey = fs.String("archive-secret-key", "", "Internet Archive secret key")
	)

	if err := ff.Parse(fs, args[1:], ff.WithEnvVars()); err != nil {
		log.Fatal("Failed to parse flags:", err)
	}

	webarchiveClient := webarchive.NewClient(*archiveAccessKey, *archiveSecretKey)
	rateLimiter := ratelimit.NewRateLimiter()

	if *replyFromEmail == "" {
		return errors.New("REPLY_FROM_EMAIL not set, which is required for email sending")
	}

	handler := &Handler{
		replyFromEmail:   *replyFromEmail,
		anthropicAPIKey:  *anthropicAPIKey,
		webarchiveClient: webarchiveClient,
		rateLimiter:      rateLimiter,

		postmarkToken:           *postmarkToken,
		postmarkWebhookUsername: *postmarkWebhookUsername,
		postmarkWebhookPassword: *postmarkWebhookPassword,
	}

	http.HandleFunc("/webhook", handler.handleInboundEmail)

	log.Printf("Server starting on %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		return fmt.Errorf("http.ListenAndServe: %w", err)
	}
	return nil
}

type Handler struct {
	replyFromEmail   string
	anthropicAPIKey  string
	webarchiveClient *webarchive.Client
	rateLimiter      *ratelimit.RateLimiter

	postmarkToken           string
	postmarkWebhookUsername string
	postmarkWebhookPassword string
}

func textResponse(w http.ResponseWriter, msg string) {
	if _, err := io.WriteString(w, msg); err != nil {
		log.Printf("failed to write text response: %v", err)
	}
}

func isIPAuthorized(ips string) bool {
	for ip := range strings.SplitSeq(ips, ",") {
		if slices.Contains(authorizedIPs, strings.TrimSpace(ip)) {
			return true
		}
	}
	return false
}

func (h *Handler) handleInboundEmail(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if !ok {
		log.Println("No basic auth in request")
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if h.postmarkWebhookUsername != user {
		log.Printf("Basic auth username %q was incorrect", user)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if h.postmarkWebhookPassword != pass {
		log.Printf("Basic auth password %q was incorrect", pass)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	requestIPs := r.Header.Get("X-Forwarded-For")
	if !isIPAuthorized(requestIPs) {
		log.Printf("None of request IPs %q was authorized", requestIPs)
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

	normalizedEmail := normalizeEmail(email.From)

	if !h.rateLimiter.IsAllowed("classification:global", 250, time.Hour) {
		log.Printf("Global classification rate limit exceeded")
		http.Error(w, "Service temporarily unavailable - too many requests", http.StatusServiceUnavailable)
		return
	}

	classification, err := claude.ClassifyPolicyChange(h.anthropicAPIKey, email.Subject, email.TextBody, email.HtmlBody)
	if err != nil {
		log.Printf("Error classifying email: %v", err)
		textResponse(w, "Classification failed")
		return
	}

	log.Printf("Classification result: isPolicyChange=%t, type=%s, company=%s, confidence=%s, policy_url=%s",
		classification.IsPolicyChange, classification.PolicyType, classification.Company, classification.Confidence, classification.PolicyURL)

	if !classification.IsPolicyChange {
		log.Printf("Email is not a policy change notification, ignoring")
		textResponse(w, "Email processed - not a policy change")
		return
	}

	if !h.rateLimiter.IsAllowed("user:"+normalizedEmail, 5, time.Hour) {
		log.Printf("Per-user rate limit exceeded for %s", normalizedEmail)
		textResponse(w, "Rate limit exceeded - please try again later")
		return
	}

	if !h.rateLimiter.IsAllowed("analysis:global", 100, time.Hour) {
		log.Printf("Global analysis rate limit exceeded")
		textResponse(w, "Service temporarily unavailable - too many requests")
		return
	}

	// Use heuristics and external APIs to come up with the policy we're looking at.
	policyResult := h.comeUpWithAPolicyURL(classification)
	if policyResult == nil {
		log.Printf("We couldn't figure out a policy URL, aborting")
		textResponse(w, "Email processed - no policy documents found - probably our fault")
		return
	}

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
	// 3. Load the previous version

	var (
		deltaReport   *templates.DeltaReport
		summaryReport *templates.SummaryReport
	)

	// Parse email date
	emailDate, err := parseEmailDate(email.Date)
	if err != nil {
		log.Printf("Failed to parse email date %q, using current date: %v", email.Date, err)
		emailDate = time.Now()
	}

	previousVersion, previousDate, snapshotURL, err := h.loadPreviousLegalDocument(emailDate, policyResult.URL)
	if err != nil {
		if errors.Is(err, errNoPreviousSnapshots) {
			log.Printf("No previous snapshots found for %q", policyResult.URL.String())
		} else {
			log.Printf("Error loading previous version: %v", err)
		}

		// We have no previous version, populate the summary report
		summaryRes, err := claude.GenerateSummaryReport(h.anthropicAPIKey, classification, policyResult.ResponseBody)
		if err != nil {
			log.Printf("Failed to generate summary report: %v", err)
		} else {
			summaryReport = &templates.SummaryReport{
				Points:    policyHighlightToSummaryPoints(summaryRes.Highlights),
				PolicyURL: policyResult.URL.String(),
				Trimmed:   summaryRes.Trimmed,
			}
		}
	}

	if previousVersion != "" {
		edits := diff.Strings(previousVersion, policyResult.ResponseBody)
		policyDiff, err := diff.ToUnified("previous-policy", "current-policy", previousVersion, edits, 20 /* context lines */)
		if err != nil {
			log.Printf("Failed to diff two policy versions (generally shouldn't happen!): %v", err)
		}

		if policyDiff != "" {
			diffSummary, err := claude.GenerateDiffReport(h.anthropicAPIKey, classification, policyDiff)
			if err != nil {
				log.Printf("Failed to generate diff report: %v", err)
			} else {
				deltaReport = &templates.DeltaReport{
					PrevDate: previousDate.Format(time.DateOnly),
					PrevURL:  snapshotURL,
					YourDate: emailDate.Format(time.DateOnly),
					YourURL:  policyResult.URL.String(),
					Points:   diffHighlightToSummaryPoints(diffSummary.Highlights),
					Trimmed:  diffSummary.Trimmed,
				}
			}
		}

	}

	genReq := &templates.GenerateRequest{
		Classification: classification,
		Service:        policyResult.Service,
		DeltaReport:    deltaReport,
		SummaryReport:  summaryReport,
	}

	emailContent, err := templates.GenerateEmail(genReq)
	if err != nil {
		log.Printf("Error generating HTML email: %v", err)
		textResponse(w, "Failed to generate the summary email")
		return
	}
	subject := fmt.Sprintf("Policy Change Summary: %s", classification.Company)

	if !h.rateLimiter.IsAllowed("email:global", 1000, time.Hour) {
		log.Printf("Global email sending rate limit exceeded")
		textResponse(w, "Service temporarily unavailable - email sending limit reached")
		return
	}

	messageID := postmark.GetMessageIDFromHeaders(&email)
	err = postmark.SendEmailWithThreading(h.postmarkToken, h.replyFromEmail, email.From, subject, emailContent.TextBody, emailContent.HTMLBody, messageID, messageID)
	if err != nil {
		log.Printf("Error sending summary email: %v", err)
		textResponse(w, "Failed to send the summary email")
		return
	}

	log.Printf("Summary email sent to %s", email.From)

	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "Policy change email processed successfully"); err != nil {
		log.Printf("failed to write text response: %v", err)
	}
}

type PolicyLoadResult struct {
	URL          *url.URL
	ResponseBody string

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
				if tosDRService == nil {
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

	var result *PolicyLoadResult
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
		// We follow redirects (common in emails with trackers) to get the actual final URL
		policyContents, finalPolicyURL, err := getBody(policyURL)
		if err != nil {
			log.Printf("Strategy %q gave us a URL (%q) that we couldn't load: %v", st.name, policyURL.String(), err)
			continue
		}

		// If we're here, I think we're good!
		// The reason we structure it this way is that we want to load from ToS;DR every
		// time, even if we use the policy URL from the classification result.
		if result == nil {
			result = &PolicyLoadResult{
				URL:          finalPolicyURL,
				ResponseBody: policyContents,
			}
		}
	}
	if result != nil {
		result.Service = svc
	}

	// Nil just means we didn't find anything.
	return result
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

	// If we found a specific document, limit our points to that one
	// Otherwise we might have like a bajillion points
	if documentURL != "" {
		var points []tosdr.Point
		for _, p := range serviceDetails.Points {
			if p.Source == documentURL {
				points = append(points, p)
			}
		}
		// Only overwrite it if that worked.
		if len(points) > 0 {
			serviceDetails.Points = points
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

var errNoPreviousSnapshots = errors.New("no previous snapshots found for given URL")

func (h *Handler) loadPreviousLegalDocument(emailDate time.Time, documentURL *url.URL) (string, time.Time, string, error) {
	// Only consider snapshots from a week before the email.
	afterTS := emailDate.AddDate(0, 0, -7)

	// Get snapshots from Internet Archive
	dURL := *documentURL
	// Remove query parameters, the WebArchive API doesn't like them
	dURL.RawQuery = ""
	snapshots, err := h.webarchiveClient.GetSnapshots(dURL.String())
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("failed to get snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return "", time.Time{}, "", errNoPreviousSnapshots
	}

	// Find the most recent snapshot that's older than our target date, a bit before
	// the email was sent.
	var bestSnapshot *webarchive.Snapshot
	for _, snapshot := range snapshots {
		ts := snapshot.Timestamp

		if ts.Before(afterTS) {
			if bestSnapshot == nil || ts.After(bestSnapshot.Timestamp) {
				bestSnapshot = &snapshot
			}
		}
	}

	if bestSnapshot == nil {
		return "", time.Time{}, "", errNoPreviousSnapshots
	}

	log.Printf("Using snapshot from %s for URL %s", bestSnapshot.Timestamp, documentURL)

	content, snapshotURL, err := h.webarchiveClient.LoadSnapshot(dURL.String(), bestSnapshot.Timestamp)
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("failed to load snapshot: %w", err)
	}

	return content, bestSnapshot.Timestamp, snapshotURL, nil
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
			service.Name, service.Rating, service.ID, service.IsComprehensivelyReviewed)
	}

	return &tosDRResults.Services[0], nil
}

func getBody(u *url.URL) (string, *url.URL, error) {
	resp, err := http.Get(u.String())
	if err != nil {
		return "", nil, fmt.Errorf("failed to load %q: %w", u.String(), err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close body: %v", err)
		}
	}()

	body, err := htmlutil.ExtractText(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed to extract text from HTML body: %w", err)
	}

	return body, resp.Request.URL, nil
}

func policyHighlightToSummaryPoints(points []claude.PolicyHighlight) []templates.SummaryPoint {
	out := make([]templates.SummaryPoint, 0, len(points))
	for _, p := range points {
		out = append(out, templates.SummaryPoint{
			Text:           p.Description,
			Classification: p.Classification,
		})
	}
	return out
}

func diffHighlightToSummaryPoints(points []claude.DiffHighlight) []templates.SummaryPoint {
	out := make([]templates.SummaryPoint, 0, len(points))
	for _, p := range points {
		out = append(out, templates.SummaryPoint{
			Text:           p.Description,
			Classification: p.Classification,
		})
	}
	return out
}

func parseEmailDate(dt string) (time.Time, error) {
	formats := []string{
		"Mon, 02 Jan 2006 15:04:05 -0700", // is time.RFC1123Z, but we put it here to show all the permutations we try.
		"Mon, 2 Jan 2006 15:04:05 -0700",
	}

	var rErr error
	for _, format := range formats {
		emailDate, err := time.Parse(format, dt)
		if err != nil {
			rErr = multierror.Append(rErr, err)
			continue
		}
		return emailDate, nil
	}

	return time.Time{}, rErr
}

func normalizeEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}

	localPart := parts[0]
	domain := parts[1]

	if domain == "gmail.com" || domain == "googlemail.com" {
		localPart = strings.ReplaceAll(localPart, ".", "")
		domain = "gmail.com"
	}
	if plusIdx := strings.Index(localPart, "+"); plusIdx != -1 {
		localPart = localPart[:plusIdx]
	}

	return localPart + "@" + domain
}
