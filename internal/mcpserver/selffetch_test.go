package mcpserver

import (
	"strings"
	"testing"
)

func TestSelfFetchClauseFollowsSetting(t *testing.T) {
	if got := selfFetchClause(true); !strings.Contains(got, "works") {
		t.Errorf("on: %q", got)
	}
	if got := selfFetchClause(false); !strings.Contains(got, "fails") || strings.Contains(got, "works") {
		t.Errorf("off: %q", got)
	}
}
