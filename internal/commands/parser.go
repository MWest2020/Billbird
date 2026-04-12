package commands

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type CommandType string

const (
	CmdLog     CommandType = "log"
	CmdCorrect CommandType = "correct"
	CmdDelete  CommandType = "delete"
)

type Command struct {
	Type        CommandType
	Minutes     int
	Description string
}

var cmdPattern = regexp.MustCompile(`(?m)^/(log|correct|delete)\s*(.*)$`)
var durationPattern = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?\s*(.*)$`)

// Parse extracts the first Billbird slash command from a comment body.
// Returns nil if no command is found.
func Parse(body string) (*Command, error) {
	matches := cmdPattern.FindStringSubmatch(body)
	if matches == nil {
		return nil, nil
	}

	cmdType := CommandType(matches[1])
	args := strings.TrimSpace(matches[2])

	if cmdType == CmdDelete {
		return &Command{Type: CmdDelete}, nil
	}

	if args == "" {
		return nil, fmt.Errorf("missing duration: use /%s <duration> (e.g. /%s 2h, /%s 30m, /%s 1h30m)", cmdType, cmdType, cmdType, cmdType)
	}

	minutes, desc, err := parseDuration(args)
	if err != nil {
		return nil, err
	}

	return &Command{
		Type:        cmdType,
		Minutes:     minutes,
		Description: desc,
	}, nil
}

func parseDuration(s string) (int, string, error) {
	matches := durationPattern.FindStringSubmatch(s)
	if matches == nil {
		return 0, "", fmt.Errorf("invalid duration format: %q — use e.g. 2h, 30m, 1h30m", s)
	}

	hours := 0
	minutes := 0
	var err error

	if matches[1] != "" {
		hours, err = strconv.Atoi(matches[1])
		if err != nil {
			return 0, "", fmt.Errorf("invalid hours: %q", matches[1])
		}
	}

	if matches[2] != "" {
		minutes, err = strconv.Atoi(matches[2])
		if err != nil {
			return 0, "", fmt.Errorf("invalid minutes: %q", matches[2])
		}
	}

	total := hours*60 + minutes
	if total == 0 {
		return 0, "", fmt.Errorf("duration must be greater than zero")
	}

	desc := strings.TrimSpace(matches[3])
	return total, desc, nil
}

// FormatDuration formats minutes as a human-readable duration string.
func FormatDuration(minutes int) string {
	h := minutes / 60
	m := minutes % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}
