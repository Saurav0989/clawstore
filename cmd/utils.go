package cmd

import (
	"fmt"
	"time"

	"github.com/saurav0989/clawstore/util"
)

func parseSince(input string) (*time.Time, error) {
	t, err := util.ParseSince(input, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid duration: %w", err)
	}
	return t, nil
}

func unixToLocal(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	return time.Unix(ts, 0).Local().Format(time.RFC3339)
}
