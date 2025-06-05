package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
		Type  string          `json:"type"`
		Input json.RawMessage `json:"input"`
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
</examples>

<company>%s</company>

<policy_url>%s</policy_url>

<policy_type>%s</policy_type>

<document_to_analyze>
%s
</document_to_analyze>
`, pc.Company, pc.PolicyURL, pc.PolicyType, textBody)

	reqBody := &Request{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 10000,
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
	if len(unifiedDiff) > 50000 {
		log.Printf("Trimming unified diff, which is %d bytes long", len(unifiedDiff))
		unifiedDiff = unifiedDiff[:50000]
	}

	prompt := fmt.Sprintf(`Analyze the unified diff of previous and current versions of the company document and explain the changes as a series of points. Some guidelines:

- Focus on changes that are important to an end-user, e.g. changes to data collection and tracking
- Don't mention things that aren't changing, where the policy is the functionally the same, even if the wording is different
- DO NOT mention any diffs that involve links changing from Web Archive to the company's site
	- That's an artifact of our analysis pipeline and SHOULD NOT be mentioned to the user.
- Write in a clear and accessible way, avoiding legal jargon
- If it makes sense to reference a section when talking about a change, reference it at the end

<company>%s</company>

<policy_url>%s</policy_url>

<policy_type>%s</policy_type>

<diff_to_analyze>
%s
</diff_to_analyze>
`, pc.Company, pc.PolicyURL, pc.PolicyType, unifiedDiff)

	reqBody := &Request{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 10000,
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
							Description: "Valid HTTP(S) URL where the policy can be accessed, leave blank if none is found in the email",
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

	var result T
	if err := json.Unmarshal(claudeResp.Content[0].Input, &result); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %v", err)
	}

	return &result, nil
}
