package codexhistory

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func TimeFromUnixish(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	if v > 1_000_000_000_000 {
		return time.UnixMilli(v)
	}
	return time.Unix(v, 0)
}

func ParseSince(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if strings.HasSuffix(value, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil || n < 0 {
			return time.Time{}, fmt.Errorf("invalid day range %q", value)
		}
		return now.AddDate(0, 0, -n), nil
	}
	t, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid since %q, use 30d or YYYY-MM-DD", value)
	}
	return t, nil
}
