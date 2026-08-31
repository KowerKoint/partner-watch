package main

import "testing"

func TestNormalizeServerURL(t *testing.T) {
	got, err := normalizeServerURL(" https://watch.example.com/ ")
	if err != nil || got != "https://watch.example.com" {
		t.Fatalf("normalizeServerURL = %q, %v", got, err)
	}
	for _, invalid := range []string{"", "http://watch.example.com", "https://watch.example.com/path", "https://user@watch.example.com"} {
		if _, err := normalizeServerURL(invalid); err == nil {
			t.Errorf("normalizeServerURL(%q) succeeded", invalid)
		}
	}
}
