package linuxnotify

import "testing"

func TestPlainStripsSupportedNotificationMarkup(t *testing.T) {
	if got := plain(" <b>Hello</b> &amp; <a href='x'>world</a> "); got != "Hello & world" {
		t.Fatalf("plain=%q", got)
	}
}

func TestTruncateUsesRunes(t *testing.T) {
	if got := truncate("あいうえ", 3); got != "あいう" {
		t.Fatalf("truncate=%q", got)
	}
}
