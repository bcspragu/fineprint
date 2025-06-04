package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema JSONSchema `json:"input_schema"`
}

type ToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type Request struct {
	Model      string      `json:"model"`
	MaxTokens  int         `json:"max_tokens,omitempty"`
	Messages   []Message   `json:"messages"`
	Tools      []Tool      `json:"tools"`
	ToolChoice *ToolChoice `json:"tool_choice,omitempty"`
}

type Response struct {
	Content []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"content"`
}

type PolicyClassification struct {
	IsPolicyChange bool   `json:"is_policy_change"`
	PolicyType     string `json:"policy_type"`
	Company        string `json:"company"`
	Confidence     string `json:"confidence"`
	PolicyURL      string `json:"policy_url"`
}

type PolicySummary struct {
	Highlights []string `json:"highlights"`
}

type DiffSummary struct {
	Highlights []string `json:"highlights"`
}

func GenerateSummaryReport(apiKey string, pc *PolicyClassification, textBody string) (*PolicySummary, error) {
	prompt := fmt.Sprintf(`Analyze the text of the provided company document and highlight the details that are important to an end-user as a series of points, here are some examples from the ToS;DR service describing PayPal's various user agreements:

<examples>
- "This service allows you to retrieve an archive of your data"
- "This service ignores the Do Not Track (DNT) header and tracks users anyway even if they set this header."
- "The service may use tracking pixels, web beacons, browser fingerprinting, and/or device fingerprinting on users."
- "The service may change its terms at any time, but the user will receive notification of the changes."
- "This service requires first-party cookies"
- "This service holds onto content that you've deleted"
- "The service informs users that its privacy policy does not apply to third party websites"
- "Third parties used by the service are bound by confidentiality obligations"
- "You can limit how your information is used by third-parties and the service"
- "This service may use your personal information for marketing purposes"
- "The service uses social media cookies/pixels"
- "Blocking first party cookies may limit your ability to use the service"
- "The service provides information about how they intend to use your personal data"
- "This service collects your IP address for location use"
- "You can opt out of promotional communications"
- "This service can share your personal information to third parties "
- "The service provides information about how they collect personal data"
- "The service reviews its privacy policy on a regular basis"
- "The service will only respond to government requests that are reasonable"
- "This service receives your location through GPS coordinates"
- "Your profile is combined across various products"
- "The service provides details about what kinds of personal information they collect"
- "This service employs separate policies for different parts of the service"
- "This service is only available to users over a certain age"
- "Users agree not to use the service for illegal purposes"
- "Instead of asking directly, this Service will assume your consent merely from your usage."
- "This service does not force users into binding arbitration"
- "Any liability on behalf of the service is limited to $10 000"
- "You waive your right to a class action."
- "This service can use your content for all their existing and future services"
- "You maintain ownership of your content"
- "You waive your moral rights"
- "Third-party cookies are used for advertising"
- "Do Not Track (DNT) headers are ignored and you are tracked anyway even if you set this header."
- "You must create an account to use this service"
- "Certain features maybe unavailable, depending on when you opened an account"
- "Your information is only shared with third parties when given specific consent"
- "Your account can be closed for several reasons"
- "You will be notified about discontinuation of service(s), unless not possible"
- "Two months of notice are given before closing your account"
- "You have the right to leave this service at any time"
- "Your data may be processed and stored anywhere in the world"
- "You must provide your identifiable information"
- "When the service wants to make a material change to its terms, you are notified at least 30 days in advance"
- "There is a date of the last update of the agreements"
- "You are responsible for maintaining the security of your account and for the activities on your account"
- "Blocking cookies may limit your ability to use the service"
- "Third parties may be involved in operating the service"
- "Your biometric data is collected"
- "The service uses your personal data to employ targeted third-party advertising"
- "You can request access and deletion of personal data"
- "This service uses third-party cookies for statistics"
- "This service gathers information about you through third parties"
- "The service collects many different types of personal data"
- "This service may keep personal data after a request for erasure for business interests or legal obligations"
- "You agree to defend, indemnify, and hold the service harmless in case of a claim related to your use of the service"
- "This service still tracks you even if you opted out from tracking"
</examples>

<company>%s</company>

<policy_url>%s</policy_url>

<policy_type>%s</policy_type>

<document_to_analyze>
%s
</document_to_analyze>
`, pc.Company, pc.PolicyURL, pc.PolicyType, textBody)

	reqBody := &Request{
		Model: "claude-sonnet-4-20250514",
		Tools: []Tool{
			{
				Name:        "extract_highlights",
				Description: "Analyze the text of a company's user-facing legal documents and extract highlights that will be important to users",
				InputSchema: JSONSchema{
					Type: ObjectType,
					Properties: map[string]*JSONSchema{
						"highlights": {
							Type: ArrayType,
							Items: &JSONSchema{
								Type:        StringType,
								Description: "An individual highlight to show to a user, ex 'The service collects many different types of personal data'",
							},
						},
					},
				},
			},
		},
		ToolChoice: &ToolChoice{
			Type: "tool",
			Name: "extract_highlights",
		},
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	return issueRequest[PolicySummary](apiKey, reqBody)
}

func GenerateDiffReport(apiKey string, pc *PolicyClassification, unifiedDiff string) (*DiffSummary, error) {
	prompt := fmt.Sprintf(`Analyze the unified diff of previous and current versions of the company document and explain the changes (focusing on those that are important to an end-user) as a series of points:

<company>%s</company>

<policy_url>%s</policy_url>

<policy_type>%s</policy_type>

<diff_to_analyze>
%s
</diff_to_analyze>
`, pc.Company, pc.PolicyURL, pc.PolicyType, unifiedDiff)

	reqBody := &Request{
		Model: "claude-sonnet-4-20250514",
		Tools: []Tool{
			{
				Name:        "extract_highlights",
				Description: "Analyze the unified diff between two versions of a company's user-facing legal documents and extract highlights that will be important to users",
				InputSchema: JSONSchema{
					Type: ObjectType,
					Properties: map[string]*JSONSchema{
						"highlights": {
							Type: ArrayType,
							Items: &JSONSchema{
								Type:        StringType,
								Description: "An individual highlight to show to a user, ex 'The service now stores your data for 30 days (up from 7 days)'",
							},
						},
					},
				},
			},
		},
		ToolChoice: &ToolChoice{
			Type: "tool",
			Name: "extract_highlights",
		},
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	return issueRequest[DiffSummary](apiKey, reqBody)
}

func ClassifyPolicyChange(apiKey, subject, textBody, htmlBody string) (*PolicyClassification, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not provided")
	}

	emailContent := ""
	if textBody != "" {
		emailContent += fmt.Sprintf("Text Body:\n%s\n\n", textBody)
	}
	if htmlBody != "" {
		emailContent += fmt.Sprintf("HTML Body:\n%s\n\n", htmlBody)
	}
	if emailContent == "" {
		emailContent = "No email body content available"
	}

	prompt := fmt.Sprintf(`Analyze this email to determine if it's a company notifying about policy changes (Terms of Service, Privacy Policy, User Agreement, etc.).

Subject: %s

%s`, subject, emailContent)

	// Old prompt before tool call
	_ = `Respond with only a JSON object in this format:
	{
	  "is_policy_change": true/false,
	  "policy_type": "terms_of_service" | "privacy_policy" | "user_agreement" | "other" | "",
	  "company": "company name or empty string",
	  "confidence": "high" | "medium" | "low",
	  "policy_url": "string"
	}`

	reqBody := &Request{
		Model:     "claude-3-5-haiku-20241022",
		MaxTokens: 600,
		Tools: []Tool{
			{
				Name:        "classify_email",
				Description: "Analyze the body of a given email to determine if it's a company notifying about a policy or legal agreement change",
				InputSchema: JSONSchema{
					Type: ObjectType,
					Properties: map[string]*JSONSchema{
						"is_policy_change": {
							Type:        BooleanType,
							Description: "True if this is indeed a company notifying about some policy or legal agreement change",
						},
						"policy_type": {
							Type:        StringType,
							Description: "The high-level type of the policy that has been updated",
							Enum:        []any{"terms_of_service", "privacy_policy", "user_agreement", "other", ""},
						},
						"company": {
							Type:        StringType,
							Description: "The name of the company who's policy has changed",
						},
						"confidence": {
							Type:        StringType,
							Description: "Level of confidence that this email does indeed indicate that some agreement/policy is changing",
							Enum:        []any{"high", "medium", "low"},
						},
						"policy_url": {
							Type:        StringType,
							Description: "URL where the updated policy can be accessed",
						},
					},
					Required: []string{
						"is_policy_change", "policy_type", "company", "confidence", "policy_url",
					},
				},
			},
		},
		ToolChoice: &ToolChoice{
			Type: "tool",
			Name: "classify_email",
		},
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	return issueRequest[PolicyClassification](apiKey, reqBody)
}

func issueRequest[T any](apiKey string, apiReq *Request) (*T, error) {
	jsonData, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close response body for Anthropic API request: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude API returned status: %d", resp.StatusCode)
	}

	var claudeResp Response
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return nil, fmt.Errorf("error decoding Claude response: %v", err)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from Claude")
	}

	responseText := claudeResp.Content[0].Text
	responseText = strings.TrimSpace(responseText)

	var result T
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("error parsing classification JSON: %v", err)
	}

	return &result, nil
}
