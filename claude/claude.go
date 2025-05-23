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
	MaxTokens  int         `json:"max_tokens"`
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

	reqBody := Request{
		Model:     "claude-3-5-haiku-20241022",
		MaxTokens: 200,
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

	jsonData, err := json.Marshal(reqBody)
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

	var classification PolicyClassification
	if err := json.Unmarshal([]byte(responseText), &classification); err != nil {
		return nil, fmt.Errorf("error parsing classification JSON: %v", err)
	}

	return &classification, nil
}
