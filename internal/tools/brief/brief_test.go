package brief

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/observe"
)

type staticState struct{ dir string }

func (s staticState) WorkDir() string { return s.dir }

func TestInvoke_ValidMessage(t *testing.T) {
	bus := observe.NewEventBus(100)
	tl := &Tool{Bus: bus}

	input, _ := json.Marshal(BriefInput{Message: "Hello user"})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Content != "Message delivered to user." {
		t.Errorf("Content = %q, want 'Message delivered to user.'", result.Content)
	}
}

func TestInvoke_EmptyMessage(t *testing.T) {
	tl := &Tool{}
	input, _ := json.Marshal(BriefInput{Message: ""})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Content != "Message is required." {
		t.Errorf("Content = %q, want 'Message is required.'", result.Content)
	}
}

func TestInvoke_WithAttachments(t *testing.T) {
	tmp := t.TempDir()
	bus := observe.NewEventBus(100)

	// Create test files
	testFile := filepath.Join(tmp, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0644)
	imgFile := filepath.Join(tmp, "screenshot.png")
	os.WriteFile(imgFile, []byte("fake-png"), 0644)

	tl := &Tool{Bus: bus}
	input, _ := json.Marshal(BriefInput{
		Message:     "Check these files",
		Attachments: []string{"test.txt", "screenshot.png"},
	})

	result, err := tl.Invoke(context.Background(), input, staticState{tmp})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "2 attachments included") {
		t.Errorf("Content = %q, want '2 attachments included'", result.Content)
	}
}

func TestInvoke_SingleAttachment(t *testing.T) {
	tmp := t.TempDir()
	bus := observe.NewEventBus(100)

	testFile := filepath.Join(tmp, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0644)

	tl := &Tool{Bus: bus}
	input, _ := json.Marshal(BriefInput{
		Message:     "Check this file",
		Attachments: []string{"test.txt"},
	})

	result, err := tl.Invoke(context.Background(), input, staticState{tmp})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "1 attachment included") {
		t.Errorf("Content = %q, want '1 attachment included'", result.Content)
	}
}

func TestInvoke_NonexistentAttachment(t *testing.T) {
	tmp := t.TempDir()
	tl := &Tool{}
	input, _ := json.Marshal(BriefInput{
		Message:     "Check this",
		Attachments: []string{"doesnotexist.txt"},
	})
	result, err := tl.Invoke(context.Background(), input, staticState{tmp})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// Non-existent attachment is recorded as error but message still delivered
	if result.Content != "Message delivered to user." {
		t.Errorf("Content = %q, want 'Message delivered to user.'", result.Content)
	}
}

func TestResolveAttachment(t *testing.T) {
	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0644)
	imgFile := filepath.Join(tmp, "photo.png")
	os.WriteFile(imgFile, []byte("fake"), 0644)
	subDir := filepath.Join(tmp, "subdir")
	os.Mkdir(subDir, 0755)

	tests := []struct {
		name    string
		rawPath string
		wantErr string
		isImage bool
	}{
		{"relative path", "test.txt", "", false},
		{"absolute path", testFile, "", false},
		{"image file", "photo.png", "", true},
		{"path traversal", "../../etc/passwd", "outside working directory", false},
		{"nonexistent", "missing.txt", "does not exist", false},
		{"directory", "subdir", "directory, not a file", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := resolveAttachment(tt.rawPath, tmp)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.IsImage != tt.isImage {
				t.Errorf("IsImage = %v, want %v", info.IsImage, tt.isImage)
			}
		})
	}
}

func TestImageExtensions(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".png", true},
		{".jpg", true},
		{".jpeg", true},
		{".gif", true},
		{".webp", true},
		{".svg", true},
		{".txt", false},
		{".go", false},
		{".pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := imageExtensions[tt.ext]
			if got != tt.want {
				t.Errorf("imageExtensions[%q] = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}
