package templates

import (
	"bytes"
	"embed"
	"fmt"
	"os/exec"
	"strings"
	"text/template"

	"postmark-inbound/claude"
	"postmark-inbound/tosdr"
)

//go:embed *.mjml
var templateFiles embed.FS

type EmailTemplateData struct {
	Subject       string
	Company       string
	PolicyType    string
	Confidence    string
	HasToSDR      bool
	ToSDRServices []tosdr.SearchService
}

func GenerateMJML(classification *claude.PolicyClassification, tosDRResults *tosdr.SearchResponse) (string, error) {
	tmpl, err := template.ParseFS(templateFiles, "email.mjml")
	if err != nil {
		return "", fmt.Errorf("error parsing template: %v", err)
	}

	data := EmailTemplateData{
		Subject:    fmt.Sprintf("Policy Change Summary: %s", classification.Company),
		Company:    classification.Company,
		PolicyType: strings.Title(strings.ReplaceAll(classification.PolicyType, "_", " ")),
		Confidence: strings.Title(classification.Confidence),
		HasToSDR:   tosDRResults != nil && len(tosDRResults.Services) > 0,
	}

	if data.HasToSDR {
		data.ToSDRServices = tosDRResults.Services
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template: %v", err)
	}

	return buf.String(), nil
}

func CompileMJMLToHTML(mjmlContent string) (string, error) {
	cmd := exec.Command("node", "mjml/compile-mjml.js", mjmlContent)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error running MJML compiler: %v, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

func GenerateHTMLEmail(classification *claude.PolicyClassification, tosDRResults *tosdr.SearchResponse) (string, error) {
	mjmlContent, err := GenerateMJML(classification, tosDRResults)
	if err != nil {
		return "", fmt.Errorf("error generating MJML: %v", err)
	}

	htmlContent, err := CompileMJMLToHTML(mjmlContent)
	if err != nil {
		return "", fmt.Errorf("error compiling MJML to HTML: %v", err)
	}

	return htmlContent, nil
}
