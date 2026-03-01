package util

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var durationRE = regexp.MustCompile(`^\s*(\d+)\s*([smhdwy])\s*$`)

func ParseDuration(input string) (time.Duration, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return 0, fmt.Errorf("duration is empty")
	}
	m := durationRE.FindStringSubmatch(input)
	if len(m) != 3 {
		return 0, fmt.Errorf("invalid duration %q; use formats like 10m, 1h, 7d, 1y", input)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid duration %q", input)
	}
	switch m[2] {
	case "s":
		return time.Duration(n) * time.Second, nil
	case "m":
		return time.Duration(n) * time.Minute, nil
	case "h":
		return time.Duration(n) * time.Hour, nil
	case "d":
		return time.Duration(n) * 24 * time.Hour, nil
	case "w":
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case "y":
		return time.Duration(n) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid duration unit in %q", input)
	}
}

func ParseSince(input string, now time.Time) (*time.Time, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}
	dur, err := ParseDuration(input)
	if err != nil {
		return nil, err
	}
	t := now.Add(-dur)
	return &t, nil
}
