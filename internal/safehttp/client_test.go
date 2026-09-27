package safehttp

import "testing"

func TestValidatePublicURL(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/video.mkv",
		"http://localhost/video.mkv",
		"http://10.0.0.1/video.mkv",
		"file:///tmp/video.mkv",
	} {
		if err := ValidatePublicURL(raw); err == nil {
			t.Fatalf("expected %q to be blocked", raw)
		}
	}
	if err := ValidatePublicURL("https://cdn.example/video.mkv?token=abc"); err != nil {
		t.Fatalf("public URL rejected: %v", err)
	}
}
