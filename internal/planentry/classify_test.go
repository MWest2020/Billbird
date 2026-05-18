package planentry

import "testing"

func TestClassifyStatus(t *testing.T) {
	cases := []struct {
		name    string
		planned int
		logged  int
		want    string
	}{
		{"no plan", 0, 120, "no_plan"},
		{"no plan zero logged", 0, 0, "no_plan"},
		{"perfectly on target", 480, 480, "on_target"},
		{"under by 1h on 8h plan", 480, 420, "under"},
		{"over by 1h on 8h plan", 480, 540, "over"},
		{"within 5 percent over", 480, 500, "on_target"},
		{"within 5 percent under", 480, 460, "on_target"},
		{"just over 5 percent over", 480, 510, "over"},
		{"just under 5 percent under", 480, 450, "under"},
		{"small plan within tolerance", 60, 62, "on_target"},
		{"small plan over by 10 minutes", 60, 70, "over"},
		{"tiny plan honours minimum 1-minute tolerance", 10, 11, "on_target"},
		{"tiny plan over by 2 minutes", 10, 12, "over"},
		{"zero logged with active plan", 240, 0, "under"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyStatus(c.planned, c.logged)
			if got != c.want {
				t.Errorf("classifyStatus(planned=%d, logged=%d) = %q, want %q",
					c.planned, c.logged, got, c.want)
			}
		})
	}
}
