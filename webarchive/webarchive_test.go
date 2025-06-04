package webarchive

import (
	"io"
	"strings"
	"testing"
)

func TestWaybackToolbarStripper_BasicRemoval(t *testing.T) {
	input := `<html>
<head><title>Test</title></head>
<body>
<h1>Before toolbar</h1>
<!-- BEGIN WAYBACK TOOLBAR INSERT -->
<div id="wm-toolbar">Wayback toolbar content here</div>
<!-- END WAYBACK TOOLBAR INSERT -->
<h1>After toolbar</h1>
</body>
</html>`

	expected := `<html>
<head><title>Test</title></head>
<body>
<h1>Before toolbar</h1>

<h1>After toolbar</h1>
</body>
</html>`

	reader := strings.NewReader(input)
	stripper := newWaybackToolbarStripper(reader)

	result, err := io.ReadAll(stripper)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(result) != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, string(result))
	}
}

func TestWaybackToolbarStripper_NoToolbar(t *testing.T) {
	input := `<html>
<head><title>Test</title></head>
<body>
<h1>No toolbar here</h1>
</body>
</html>`

	reader := strings.NewReader(input)
	stripper := newWaybackToolbarStripper(reader)

	result, err := io.ReadAll(stripper)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(result) != input {
		t.Errorf("Expected input to remain unchanged when no toolbar present")
	}
}

func TestWaybackToolbarStripper_MultipleToolbars(t *testing.T) {
	input := `<html>
<body>
<h1>Start</h1>
<!-- BEGIN WAYBACK TOOLBAR INSERT -->
<div>First toolbar</div>
<!-- END WAYBACK TOOLBAR INSERT -->
<h1>Middle</h1>
<!-- BEGIN WAYBACK TOOLBAR INSERT -->
<div>Second toolbar</div>
<!-- END WAYBACK TOOLBAR INSERT -->
<h1>End</h1>
</body>
</html>`

	expected := `<html>
<body>
<h1>Start</h1>

<h1>Middle</h1>

<h1>End</h1>
</body>
</html>`

	reader := strings.NewReader(input)
	stripper := newWaybackToolbarStripper(reader)

	result, err := io.ReadAll(stripper)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(result) != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, string(result))
	}
}

func TestWaybackToolbarStripper_PartialMarkers(t *testing.T) {
	input := `<html>
<body>
<h1>Test</h1>
<!-- BEGIN WAYBACK TOOLBAR -->
<p>This looks like a toolbar marker but isn't complete</p>
<!-- END WAYBACK -->
<h1>End</h1>
</body>
</html>`

	reader := strings.NewReader(input)
	stripper := newWaybackToolbarStripper(reader)

	result, err := io.ReadAll(stripper)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(result) != input {
		t.Errorf("Expected input to remain unchanged when only partial markers present")
	}
}

func TestWaybackToolbarStripper_SmallReads(t *testing.T) {
	input := `<html><body><h1>Before</h1><!-- BEGIN WAYBACK TOOLBAR INSERT --><div>toolbar</div><!-- END WAYBACK TOOLBAR INSERT --><h1>After</h1></body></html>`
	expected := `<html><body><h1>Before</h1><h1>After</h1></body></html>`

	reader := strings.NewReader(input)
	stripper := newWaybackToolbarStripper(reader)

	// Read in very small chunks to test streaming behavior
	var result strings.Builder
	buf := make([]byte, 1) // Read 1 byte at a time
	for {
		n, err := stripper.Read(buf)
		if n > 0 {
			result.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
	}

	if result.String() != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, result.String())
	}
}

func TestWaybackToolbarStripper_ToolbarAtEnd(t *testing.T) {
	input := `<html>
<body>
<h1>Content</h1>
<!-- BEGIN WAYBACK TOOLBAR INSERT -->
<div>Toolbar at end</div>
<!-- END WAYBACK TOOLBAR INSERT -->`

	expected := `<html>
<body>
<h1>Content</h1>
`

	reader := strings.NewReader(input)
	stripper := newWaybackToolbarStripper(reader)

	result, err := io.ReadAll(stripper)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(result) != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, string(result))
	}
}

func TestWaybackToolbarStripper_ToolbarAtStart(t *testing.T) {
	input := `<!-- BEGIN WAYBACK TOOLBAR INSERT -->
<div>Toolbar at start</div>
<!-- END WAYBACK TOOLBAR INSERT -->
<html>
<body>
<h1>Content</h1>
</body>
</html>`

	expected := `
<html>
<body>
<h1>Content</h1>
</body>
</html>`

	reader := strings.NewReader(input)
	stripper := newWaybackToolbarStripper(reader)

	result, err := io.ReadAll(stripper)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(result) != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, string(result))
	}
}

func TestWaybackToolbarStripper_EmptyInput(t *testing.T) {
	input := ""
	reader := strings.NewReader(input)
	stripper := newWaybackToolbarStripper(reader)

	result, err := io.ReadAll(stripper)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(result) != input {
		t.Errorf("Expected empty input to remain empty")
	}
}

func TestWaybackToolbarStripper_NestedComments(t *testing.T) {
	input := `<html>
<body>
<h1>Start</h1>
<!-- BEGIN WAYBACK TOOLBAR INSERT -->
<!-- This is a comment inside the toolbar -->
<div>Toolbar content</div>
<!-- Another comment -->
<!-- END WAYBACK TOOLBAR INSERT -->
<h1>End</h1>
</body>
</html>`

	expected := `<html>
<body>
<h1>Start</h1>

<h1>End</h1>
</body>
</html>`

	reader := strings.NewReader(input)
	stripper := newWaybackToolbarStripper(reader)

	result, err := io.ReadAll(stripper)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(result) != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, string(result))
	}
}