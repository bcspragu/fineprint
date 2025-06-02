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

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

//go:embed *.mjml
var templateFiles embed.FS

type EmailTemplateData struct {
	Subject    string
	Company    string
	PolicyType string

	DeltaReport   *DeltaReport
	SummaryReport *SummaryReport
	ToSDR         *ToSDR
}

type DeltaReport struct {
	PrevDate string
	YourDate string

	Points []SummaryPoint
}

type SummaryReport struct {
	Points []SummaryPoint
}

type SummaryPoint struct {
	Text string
}

type ToSDR struct {
	Points []ToSDRPoint
}

type ToSDRPoint struct {
	Title          string
	Source         string
	Classification string
}

func GenerateMJML(classification *claude.PolicyClassification, svc *tosdr.Service) (string, error) {
	tmpl, err := template.ParseFS(templateFiles, "email.mjml")
	if err != nil {
		return "", fmt.Errorf("error parsing template: %v", err)
	}

	title := cases.Title(language.English)

	data := EmailTemplateData{
		Subject:       fmt.Sprintf("Policy Change Summary: %s", classification.Company),
		Company:       classification.Company,
		PolicyType:    title.String(strings.ReplaceAll(classification.PolicyType, "_", " ")),
		DeltaReport:   &DeltaReport{},   // TODO: Pass in the right stuff so we know when to show this
		SummaryReport: &SummaryReport{}, // TODO: Pass in the right stuff so we know when to show this
		ToSDR:         toToSDR(svc),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template: %v", err)
	}

	return buf.String(), nil
}

func toToSDR(svc *tosdr.Service) *ToSDR {
	if svc == nil {
		return nil
	}

	points := make([]ToSDRPoint, 0, len(svc.Points))
	for _, p := range svc.Points {
		points = append(points, ToSDRPoint{
			Title:          p.Title,
			Source:         p.Source,
			Classification: p.Case.Classification,
		})
	}

	return &ToSDR{
		Points: points,
	}
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
