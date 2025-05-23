package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/peterbourgon/ff/v3"

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

func (h *Handler) handleInboundEmail(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Classification failed", http.StatusInternalServerError)
		return
	}

	log.Printf("Classification result: isPolicyChange=%t, type=%s, company=%s, confidence=%s",
		classification.IsPolicyChange, classification.PolicyType, classification.Company, classification.Confidence)

	if !classification.IsPolicyChange {
		log.Printf("Email is not a policy change notification, ignoring")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Email processed - not a policy change")
		return
	}

	log.Printf("Detected policy change from %s (type: %s)", classification.Company, classification.PolicyType)

	var tosDRResults *tosdr.SearchResponse
	var previousVersion string

	if classification.Company != "" {
		var err error
		tosDRResults, err = tosdr.SearchServices(classification.Company)
		if err != nil {
			log.Printf("Error searching ToS;DR for %s: %v", classification.Company, err)
			return
		} else {
			if len(tosDRResults.Services) > 0 {
				log.Printf("Found %d ToS;DR services for %s:", len(tosDRResults.Services), classification.Company)
				for _, service := range tosDRResults.Services {
					log.Printf("  - %s (Rating: %s, ID: %d, Comprehensive: %t)",
						service.Name, service.Rating, service.ID, service.IsComprehensive)
				}

				// Try to get legal document from first service and load previous version
				if len(tosDRResults.Services) > 0 {
					previousVersion, err = loadPreviousLegalDocument(&tosDRResults.Services[0], &email, h.webarchiveClient, classification.PolicyType)
					if err != nil {
						log.Printf("Error loading previous version: %v", err)
					} else if previousVersion != "" {
						log.Printf("Successfully loaded previous version (%d chars)", len(previousVersion))
					}
				}
			} else {
				log.Printf("No ToS;DR services found for %s", classification.Company)
			}
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
	fmt.Fprintf(w, "Policy change email processed successfully")
}

func generateSummary(classification *claude.PolicyClassification, tosDRResults *tosdr.SearchResponse) string {
	var textBody strings.Builder

	textBody.WriteString(fmt.Sprintf("Policy Change Summary for %s\n", classification.Company))
	textBody.WriteString("=" + strings.Repeat("=", len("Policy Change Summary for "+classification.Company)) + "\n\n")

	textBody.WriteString(fmt.Sprintf("Policy Type: %s\n", strings.Title(strings.ReplaceAll(classification.PolicyType, "_", " "))))
	textBody.WriteString(fmt.Sprintf("Classification Confidence: %s\n\n", strings.Title(classification.Confidence)))

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

func loadPreviousLegalDocument(service *tosdr.SearchService, email *postmark.InboundEmail, webarchiveClient *webarchive.Client, policyType string) (string, error) {
	// Parse email date
	emailDate, err := time.Parse(time.RFC1123Z, email.Date)
	if err != nil {
		return "", fmt.Errorf("failed to parse email date: %w", err)
	}

	// Get service details to find document URLs
	serviceDetails, err := tosdr.GetService(service.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get service details: %w", err)
	}

	// Find the appropriate document URL based on policy type
	var documentURL string
	for _, doc := range serviceDetails.Documents {
		docName := strings.ToLower(doc.Name)
		if policyType == "privacy_policy" && (strings.Contains(docName, "privacy") || strings.Contains(docName, "data")) {
			documentURL = doc.URL
			break
		} else if policyType == "terms_of_service" && (strings.Contains(docName, "terms") || strings.Contains(docName, "service")) {
			documentURL = doc.URL
			break
		}
	}

	// If no specific match, try the first document
	if documentURL == "" && len(serviceDetails.Documents) > 0 {
		documentURL = serviceDetails.Documents[0].URL
	}

	if documentURL == "" {
		return "", fmt.Errorf("no document URL found for policy type %s", policyType)
	}

	log.Printf("Found document URL: %s", documentURL)

	// Get snapshots from Internet Archive
	snapshots, err := webarchiveClient.GetSnapshots(documentURL)
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
