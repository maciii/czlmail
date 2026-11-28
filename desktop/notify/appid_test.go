package notify

import (
	"regexp"
	"testing"
)

func TestActivatorGUID(t *testing.T) {
	if got := activatorGUID(defaultAppID); got != defaultActivatorGUID {
		t.Fatalf("default app must keep its registered GUID, got %s", got)
	}
	dev := activatorGUID("net.czl.mail.cs-dev")
	if dev == defaultActivatorGUID {
		t.Fatal("test instance must not share the release GUID")
	}
	if dev != activatorGUID("net.czl.mail.cs-dev") {
		t.Fatal("GUID must be stable across launches")
	}
	if dev == activatorGUID("net.czl.mail.notes-test") {
		t.Fatal("instances must get distinct GUIDs")
	}
	if !regexp.MustCompile(`^\{[0-9A-F]{8}-[0-9A-F]{4}-5[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}\}$`).MatchString(dev) {
		t.Fatalf("not a braced version 5 GUID: %s", dev)
	}
}
