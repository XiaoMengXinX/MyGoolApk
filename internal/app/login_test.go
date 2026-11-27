package app

import (
	"strings"
	"testing"
)

func TestLoginWebViewAgent(t *testing.T) {
	iosUA := "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 (#Build; Apple; iPhone15,4; iPhone 15; iOS_26.7; 26.7) +iCoolMarket/16.7.4-2611191"

	ua, suffix := loginWebViewAgent(false, iosUA)
	if ua != iosUA || suffix != "" {
		t.Fatalf("mobile = (%q, %q), want (%q, \"\")", ua, suffix, iosUA)
	}

	ua, suffix = loginWebViewAgent(true, iosUA)
	if ua != "" {
		t.Fatalf("desktop userAgent = %q, want empty so platform desktop UA is kept", ua)
	}
	if strings.Contains(suffix, "iPhone") || strings.Contains(suffix, "Mobile") {
		t.Fatalf("desktop suffix %q must not advertise iPhone/Mobile", suffix)
	}
	if !strings.Contains(suffix, "+iCoolMarket/16.7.4-2611191") {
		t.Fatalf("desktop suffix %q missing Coolapk client tag", suffix)
	}
}
