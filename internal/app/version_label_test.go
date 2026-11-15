package app

import (
	"strings"
	"testing"
)

// No page shows the old bottom-left sidebar version label (removed in v0.8.14). The version stays
// visible elsewhere: Settings > About on the home page, and the keyboard-shortcuts dialog on the
// Light Table room pages (creator and participant, including the PIN screen).
func TestSidebarVersionLabelRemoved(t *testing.T) {
	pages := renderContractStates(t)
	for state, html := range pages {
		if strings.Contains(html, "sidebar-version-tag") {
			t.Errorf("%s: still renders the sidebar version label", state)
		}
	}
	for _, state := range []string{stateHome, stateCreator, stateParticipant, stateParticipantLocked} {
		if !strings.Contains(pages[state], "v0.8.14") {
			t.Errorf("%s: the version is no longer shown anywhere on the page", state)
		}
	}
}
