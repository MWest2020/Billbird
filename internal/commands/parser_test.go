package commands

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    *Command
		wantErr bool
	}{
		{
			name: "log hours",
			body: "/log 2h",
			want: &Command{Type: CmdLog, Minutes: 120},
		},
		{
			name: "log minutes",
			body: "/log 45m",
			want: &Command{Type: CmdLog, Minutes: 45},
		},
		{
			name: "log combined",
			body: "/log 1h30m",
			want: &Command{Type: CmdLog, Minutes: 90},
		},
		{
			name: "log with description",
			body: "/log 2h Fixed authentication bug",
			want: &Command{Type: CmdLog, Minutes: 120, Description: "Fixed authentication bug"},
		},
		{
			name: "correct hours",
			body: "/correct 3h",
			want: &Command{Type: CmdCorrect, Minutes: 180},
		},
		{
			name: "correct with description",
			body: "/correct 3h Revised estimate after code review",
			want: &Command{Type: CmdCorrect, Minutes: 180, Description: "Revised estimate after code review"},
		},
		{
			name: "delete",
			body: "/delete",
			want: &Command{Type: CmdDelete},
		},
		{
			name: "no command",
			body: "Just a regular comment",
			want: nil,
		},
		{
			name: "command in middle of text",
			body: "Some text before\n/log 1h\nSome text after",
			want: &Command{Type: CmdLog, Minutes: 60},
		},
		{
			name:    "log missing duration",
			body:    "/log",
			wantErr: true,
		},
		{
			name:    "log zero duration",
			body:    "/log 0h",
			wantErr: true,
		},
		{
			name:    "correct missing duration",
			body:    "/correct",
			wantErr: true,
		},
		{
			name: "not a slash command",
			body: "/something 2h",
			want: nil,
		},
		{
			name: "inline slash not matched",
			body: "I used /log at work today",
			want: nil,
		},
		{
			name: "log large hours",
			body: "/log 12h",
			want: &Command{Type: CmdLog, Minutes: 720},
		},
		{
			name: "log only minutes large",
			body: "/log 120m",
			want: &Command{Type: CmdLog, Minutes: 120},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Errorf("expected %+v, got nil", tt.want)
				return
			}
			if got.Type != tt.want.Type {
				t.Errorf("type: got %q, want %q", got.Type, tt.want.Type)
			}
			if got.Minutes != tt.want.Minutes {
				t.Errorf("minutes: got %d, want %d", got.Minutes, tt.want.Minutes)
			}
			if got.Description != tt.want.Description {
				t.Errorf("description: got %q, want %q", got.Description, tt.want.Description)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		minutes int
		want    string
	}{
		{60, "1h"},
		{120, "2h"},
		{45, "45m"},
		{90, "1h30m"},
		{150, "2h30m"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatDuration(tt.minutes)
			if got != tt.want {
				t.Errorf("FormatDuration(%d) = %q, want %q", tt.minutes, got, tt.want)
			}
		})
	}
}
