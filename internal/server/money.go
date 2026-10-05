package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseMoneyToCents accepts "12", "12.3", "12.34" (optional leading $).
func parseMoneyToCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0, fmt.Errorf("amount required")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = strings.TrimPrefix(s, "-")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount")
	}
	dollars, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount")
	}
	var cents int64
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 0 || len(frac) > 2 {
			return 0, fmt.Errorf("invalid cents")
		}
		if len(frac) == 1 {
			frac += "0"
		}
		cents, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid cents")
		}
	}
	total := dollars*100 + cents
	if neg {
		total = -total
	}
	return total, nil
}

func formatCents(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := formatIntComma(cents/100) + fmt.Sprintf(".%02d", cents%100)
	if neg {
		return "-" + s
	}
	return s
}

func formatIntComma(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre == 0 {
		pre = 3
	}
	b.WriteString(s[:pre])
	for i := pre; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// parseUSDate accepts MM/DD/YYYY (also tolerates ISO YYYY-MM-DD).
func parseUSDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("date required")
	}
	if t, err := time.Parse("01/02/2006", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid date")
}

func formatUSDate(t time.Time) string {
	return t.UTC().Format("01/02/2006")
}
