package usage

import (
	"strings"
	"testing"
)

func TestBuildWhereClauseIncludesResolvedChannelFilter(t *testing.T) {
	where, args := buildWhereClause(LogQueryParams{
		Days: 1,
		ChannelFilter: ChannelFilter{
			AuthIndexes:  []string{"auth-1"},
			ChannelNames: []string{"legacy-name"},
			Sources:      []string{"source-1"},
		},
	})

	if !strings.Contains(where, "auth_index IN (?)") {
		t.Fatalf("expected auth_index clause, got %q", where)
	}
	if !strings.Contains(where, "channel_name IN (?)") {
		t.Fatalf("expected channel_name clause, got %q", where)
	}
	if !strings.Contains(where, "source IN (?)") {
		t.Fatalf("expected source clause, got %q", where)
	}
	if !strings.Contains(where, " OR ") {
		t.Fatalf("expected OR-combined channel filter, got %q", where)
	}
	if len(args) != 4 {
		t.Fatalf("expected cutoff + 3 filter args, got %d (%+v)", len(args), args)
	}
}
