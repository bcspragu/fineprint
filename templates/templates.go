package templates

import (
	"bytes"
	"embed"
	"fmt"
	"os/exec"
	"strings"
	"text/template"

	"github.com/bcspragu/fineprint/claude"
	"github.com/bcspragu/fineprint/tosdr"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

//go:embed *.mjml *.txt
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

	PrevURL string
	YourURL string

	Points  []SummaryPoint
	Trimmed bool
}

type SummaryReport struct {
	Points    []SummaryPoint
	PolicyURL string
	Trimmed   bool
}

type SummaryPoint struct {
	Text           string
	Classification string
}

type ToSDR struct {
	Points []ToSDRPoint
}

type ToSDRPoint struct {
	Title          string
	Source         string
	Classification string
}

type GenerateRequest struct {
	Classification *claude.PolicyClassification
	Service        *tosdr.Service
	DeltaReport    *DeltaReport
	SummaryReport  *SummaryReport
}

var title = cases.Title(language.English)

func (gr *GenerateRequest) toEmailTemplateData() *EmailTemplateData {
	return &EmailTemplateData{
		Subject:       fmt.Sprintf("Policy Change Summary: %s", gr.Classification.Company),
		Company:       gr.Classification.Company,
		PolicyType:    title.String(strings.ReplaceAll(gr.Classification.PolicyType, "_", " ")),
		DeltaReport:   gr.DeltaReport,
		SummaryReport: gr.SummaryReport,
		ToSDR:         toToSDR(gr.Service),
	}
}

func GenerateMJML(tmplData *EmailTemplateData) (string, error) {
	tmpl, err := template.ParseFS(templateFiles, "email.mjml")
	if err != nil {
		return "", fmt.Errorf("error parsing template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, tmplData); err != nil {
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

type Email struct {
	HTMLBody string
	TextBody string
}

func GenerateEmail(req *GenerateRequest) (*Email, error) {
	tmplData := req.toEmailTemplateData()
	mjmlContent, err := GenerateMJML(tmplData)
	if err != nil {
		return nil, fmt.Errorf("error generating MJML: %w", err)
	}

	htmlContent, err := CompileMJMLToHTML(mjmlContent)
	if err != nil {
		return nil, fmt.Errorf("error compiling MJML to HTML: %w", err)
	}

	textContent, err := generateTextEmail(tmplData)
	if err != nil {
		return nil, fmt.Errorf("error generating text email: %w", err)
	}

	return &Email{
		HTMLBody: htmlContent,
		TextBody: textContent,
	}, nil
}

func generateTextEmail(tmplData *EmailTemplateData) (string, error) {
	tmpl, err := template.ParseFS(templateFiles, "email.txt")
	if err != nil {
		return "", fmt.Errorf("error parsing template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, tmplData); err != nil {
		return "", fmt.Errorf("error executing template: %v", err)
	}

	return buf.String(), nil
}
