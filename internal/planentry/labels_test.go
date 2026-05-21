package planentry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanEntryJSONLabelsShape(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
		want   string
	}{
		{"empty slice marshals as []", []string{}, `"labels":[]`},
		{"nil slice still marshals as null", nil, `"labels":null`},
		{"populated", []string{"strippenkaart:acme-2026q1"}, `"labels":["strippenkaart:acme-2026q1"]`},
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

func TestLabelsForInsertCoerces(t *testing.T) {
	if got := labelsForInsert(nil); got == nil {
		t.Fatalf("nil input must become non-nil empty slice")
	}
}
