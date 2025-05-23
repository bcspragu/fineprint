package postmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type InboundEmail struct {
	From     string `json:"From"`
	FromName string `json:"FromName"`
	To       string `json:"To"`
	ToFull   []struct {
		Email string `json:"Email"`
		Name  string `json:"Name"`
	} `json:"ToFull"`
	Subject           string `json:"Subject"`
	MessageID         string `json:"MessageID"`
	ReplyTo           string `json:"ReplyTo"`
	Date              string `json:"Date"`
	MailboxHash       string `json:"MailboxHash"`
	TextBody          string `json:"TextBody"`
	HtmlBody          string `json:"HtmlBody"`
	StrippedTextReply string `json:"StrippedTextReply"`
	Tag               string `json:"Tag"`
	Headers           []struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
	} `json:"Headers"`
	Attachments []struct {
		Name          string `json:"Name"`
		Content       string `json:"Content"`
		ContentType   string `json:"ContentType"`
		ContentLength int    `json:"ContentLength"`
		ContentID     string `json:"ContentID"`
	} `json:"Attachments"`
}

type EmailRequest struct {
	From          string `json:"From"`
	To            string `json:"To"`
	Subject       string `json:"Subject"`
	TextBody      string `json:"TextBody,omitempty"`
	HtmlBody      string `json:"HtmlBody,omitempty"`
	MessageStream string `json:"MessageStream,omitempty"`
	InReplyTo     string `json:"InReplyTo,omitempty"`
	References    string `json:"References,omitempty"`
}

type EmailResponse struct {
	MessageID   string `json:"MessageID"`
	SubmittedAt string `json:"SubmittedAt"`
	To          string `json:"To"`
	ErrorCode   int    `json:"ErrorCode"`
	Message     string `json:"Message"`
}

func GetMessageIDFromHeaders(email *InboundEmail) string {
	for _, header := range email.Headers {
		if header.Name == "Message-ID" {
			return header.Value
		}
	}
	return email.MessageID
}

func SendEmail(serverToken, from, to, subject, textBody, htmlBody string) error {
	return SendEmailWithThreading(serverToken, from, to, subject, textBody, htmlBody, "", "")
}

func SendEmailWithThreading(serverToken, from, to, subject, textBody, htmlBody, inReplyTo, references string) error {
	if serverToken == "" {
		return fmt.Errorf("POSTMARK_SERVER_TOKEN not provided")
	}

	emailReq := EmailRequest{
		From:          from,
		To:            to,
		Subject:       subject,
		TextBody:      textBody,
		HtmlBody:      htmlBody,
		MessageStream: "outbound",
		InReplyTo:     inReplyTo,
		References:    references,
	}

	jsonData, err := json.Marshal(emailReq)
	if err != nil {
		return fmt.Errorf("error marshaling email request: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.postmarkapp.com/email", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Postmark-Server-Token", serverToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close body on Postmark API request: %v", err)
		}
	}()

	var postmarkResp EmailResponse
	if err := json.NewDecoder(resp.Body).Decode(&postmarkResp); err != nil {
		return fmt.Errorf("error decoding Postmark response: %v", err)
	}

	if postmarkResp.ErrorCode != 0 {
		return fmt.Errorf("error from Postmark API %d: %s", postmarkResp.ErrorCode, postmarkResp.Message)
	}

	log.Printf("Email sent successfully. MessageID: %s", postmarkResp.MessageID)
	return nil
}
