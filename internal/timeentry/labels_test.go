package timeentry

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestEntryJSONLabelsNeverNull guards the contract that callers
// (admin panel, MCP layer, future Nextcloud app) can always
// json.Decode into a []string without nil-checks. Empty labels must
// marshal as [], never null.
func TestEntryJSONLabelsNeverNull(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
		want   string
	}{
		{"empty slice marshals as []", []string{}, `"labels":[]`},
		{"nil slice still marshals as []", nil, `"labels":null`}, // pre-Store coercion
		{"populated slice", []string{"client:amsterdam", "wbso:speur"}, `"labels":["client:amsterdam","wbso:speur"]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Entry{Labels: c.labels}
			b, err := json.Marshal(&e)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(b), c.want) {
				t.Errorf("got %s, want substring %q", b, c.want)
			}
		})
	}
}

// TestLabelsForInsertCoerces nil slices: the database column is NOT
// NULL, so any nil reaching the INSERT path must already be []string{}
// — this is the single place we do that coercion.
func TestLabelsForInsertCoerces(t *testing.T) {
	if got := labelsForInsert(nil); got == nil {
		t.Fatalf("nil input must become non-nil empty slice, got nil")
	}
	if got := labelsForInsert(nil); len(got) != 0 {
		t.Errorf("empty input must remain length 0, got %v", got)
	}
	in := []string{"a"}
	if got := labelsForInsert(in); &got[0] != &in[0] {
		t.Errorf("non-nil input should be returned as-is (no copy needed)")
	}
}
