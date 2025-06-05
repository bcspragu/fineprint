package htmlutil

import (
	"strings"
	"testing"
)

func TestExtractText_BasicHTML(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "simple paragraph",
			html:     `<html><body><p>Hello world</p></body></html>`,
			expected: "Hello world",
		},
		{
			name:     "multiple paragraphs",
			html:     `<html><body><p>First paragraph</p><p>Second paragraph</p></body></html>`,
			expected: "First paragraph Second paragraph",
		},
		{
			name:     "nested elements",
			html:     `<html><body><div><p>Nested <strong>text</strong></p></div></body></html>`,
			expected: "Nested text",
		},
		{
			name:     "headings and paragraphs",
			html:     `<html><body><h1>Title</h1><p>Content</p></body></html>`,
			expected: "Title Content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.html)
			result, err := ExtractText(reader)
			if err != nil {
				t.Fatalf("ExtractText() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("ExtractText() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractText_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "empty HTML",
			html:     "",
			expected: "",
		},
		{
			name:     "malformed HTML",
			html:     `<html><body><p>unclosed paragraph</body></html>`,
			expected: "unclosed paragraph",
		},
		{
			name:     "no body tag",
			html:     `<html><head><title>No body</title></head></html>`,
			expected: "",
		},
		{
			name:     "empty body",
			html:     `<html><body></body></html>`,
			expected: "",
		},
		{
			name:     "body with only whitespace",
			html:     `<html><body>   \n\t  </body></html>`,
			expected: "\\n\\t",
		},
		{
			name:     "HTML without html tag",
			html:     `<body><p>Direct body</p></body>`,
			expected: "Direct body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.html)
			result, err := ExtractText(reader)
			if err != nil {
				t.Fatalf("ExtractText() unexpected error = %v", err)
			}

			if result != tt.expected {
				t.Errorf("ExtractText() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractText_ComplexHTML(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name: "deeply nested structure",
			html: `<html><body>
				<div class="container">
					<header>
						<h1>Main Title</h1>
						<nav><a href="#">Link</a></nav>
					</header>
					<main>
						<article>
							<h2>Article Title</h2>
							<p>First paragraph with <em>emphasis</em> and <strong>bold</strong>.</p>
							<ul>
								<li>Item one</li>
								<li>Item two</li>
							</ul>
						</article>
					</main>
				</div>
			</body></html>`,
			expected: "Main Title Link Article Title First paragraph with emphasis and bold . Item one Item two",
		},
		{
			name: "table structure",
			html: `<html><body>
				<table>
					<thead>
						<tr><th>Header 1</th><th>Header 2</th></tr>
					</thead>
					<tbody>
						<tr><td>Cell 1</td><td>Cell 2</td></tr>
						<tr><td>Cell 3</td><td>Cell 4</td></tr>
					</tbody>
				</table>
			</body></html>`,
			expected: "Header 1 Header 2 Cell 1 Cell 2 Cell 3 Cell 4",
		},
		{
			name: "form elements",
			html: `<html><body>
				<form>
					<label>Name:</label>
					<input type="text" placeholder="Enter name">
					<button>Submit</button>
				</form>
			</body></html>`,
			expected: "Name: Submit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.html)
			result, err := ExtractText(reader)
			if err != nil {
				t.Fatalf("ExtractText() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("ExtractText() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractText_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "multiple spaces",
			html:     `<html><body><p>Text    with     multiple   spaces</p></body></html>`,
			expected: "Text    with     multiple   spaces",
		},
		{
			name:     "newlines and tabs",
			html:     `<html><body><p>Text\n\twith\n\tnewlines\tand\ttabs</p></body></html>`,
			expected: "Text\\n\\twith\\n\\tnewlines\\tand\\ttabs",
		},
		{
			name:     "leading and trailing whitespace",
			html:     `<html><body><p>   Leading and trailing   </p></body></html>`,
			expected: "Leading and trailing",
		},
		{
			name: "whitespace between elements",
			html: `<html><body>
				<p>First</p>
				<p>Second</p>
				<p>Third</p>
			</body></html>`,
			expected: "First Second Third",
		},
		{
			name:     "preserved single spaces",
			html:     `<html><body><p>Word one</p> <p>Word two</p></body></html>`,
			expected: "Word one Word two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.html)
			result, err := ExtractText(reader)
			if err != nil {
				t.Fatalf("ExtractText() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("ExtractText() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractText_SpecialElements(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name: "script tags included",
			html: `<html><body>
				<p>Before script</p>
				<script>console.log('hello');</script>
				<p>After script</p>
			</body></html>`,
			expected: "Before script console.log('hello'); After script",
		},
		{
			name: "style tags included",
			html: `<html><body>
				<p>Before style</p>
				<style>body { color: red; }</style>
				<p>After style</p>
			</body></html>`,
			expected: "Before style body { color: red; } After style",
		},
		{
			name: "comments are ignored",
			html: `<html><body>
				<p>Before comment</p>
				<!-- This is a comment -->
				<p>After comment</p>
			</body></html>`,
			expected: "Before comment After comment",
		},
		{
			name: "head content ignored",
			html: `<html>
				<head>
					<title>Page Title</title>
					<meta name="description" content="Page description">
				</head>
				<body>
					<p>Body content</p>
				</body>
			</html>`,
			expected: "Body content",
		},
		{
			name: "mixed special elements",
			html: `<html><body>
				<p>Content</p>
				<script>var x = 1;</script>
				<style>.class { margin: 0; }</style>
				<noscript>No JavaScript</noscript>
				<p>More content</p>
			</body></html>`,
			expected: "Content var x = 1; .class { margin: 0; } No JavaScript More content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.html)
			result, err := ExtractText(reader)
			if err != nil {
				t.Fatalf("ExtractText() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("ExtractText() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractText_RealWorldExample(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Terms of Service</title>
	<style>
		body { font-family: Arial, sans-serif; }
		.section { margin: 20px 0; }
	</style>
</head>
<body>
	<header>
		<h1>Terms of Service</h1>
		<nav>
			<a href="#section1">Section 1</a>
			<a href="#section2">Section 2</a>
		</nav>
	</header>
	
	<main>
		<section id="section1" class="section">
			<h2>1. Acceptance of Terms</h2>
			<p>By using our service, you agree to these terms.</p>
			<ul>
				<li>You must be 18 years or older</li>
				<li>You must provide accurate information</li>
			</ul>
		</section>
		
		<section id="section2" class="section">
			<h2>2. Privacy Policy</h2>
			<p>We respect your privacy and handle data according to our <a href="/privacy">Privacy Policy</a>.</p>
		</section>
	</main>
	
	<footer>
		<p>© 2024 Company Name. All rights reserved.</p>
	</footer>
	
	<script>
		console.log('Terms loaded');
	</script>
</body>
</html>`

	expected := "Terms of Service Section 1 Section 2 1. Acceptance of Terms By using our service, you agree to these terms. You must be 18 years or older You must provide accurate information 2. Privacy Policy We respect your privacy and handle data according to our Privacy Policy . © 2024 Company Name. All rights reserved. console.log('Terms loaded');"

	reader := strings.NewReader(html)
	result, err := ExtractText(reader)
	if err != nil {
		t.Fatalf("ExtractText() error = %v", err)
	}
	if result != expected {
		t.Errorf("ExtractText() = %q, want %q", result, expected)
	}
}
