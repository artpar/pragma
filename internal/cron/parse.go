package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpr is a parsed 5-field cron expression.
// Fields: minute, hour, day-of-month, month, day-of-week.
type CronExpr struct {
	minute  [60]bool
	hour    [24]bool
	dom     [32]bool // 1-31 (index 0 unused)
	month   [13]bool // 1-12 (index 0 unused)
	dow     [7]bool  // 0-6, 0=Sunday
	domStar bool     // true if dom field was "*" (all days)
	dowStar bool     // true if dow field was "*" (all weekdays)
}

// Parse parses a 5-field cron expression.
// Syntax per field: *, N, N-M, */N, N-M/S, N,M,...
func Parse(expr string) (*CronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(fields), expr)
	}

	var c CronExpr
	if err := parseField(fields[0], c.minute[:], 0, 59); err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	if err := parseField(fields[1], c.hour[:], 0, 23); err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	c.domStar = fields[2] == "*"
	if err := parseField(fields[2], c.dom[:], 1, 31); err != nil {
		return nil, fmt.Errorf("cron day-of-month: %w", err)
	}
	if err := parseField(fields[3], c.month[:], 1, 12); err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	c.dowStar = fields[4] == "*"
	if err := parseField(fields[4], c.dow[:], 0, 6); err != nil {
		return nil, fmt.Errorf("cron day-of-week: %w", err)
	}
	return &c, nil
}

func parseField(field string, bits []bool, lo, hi int) error {
	// Handle comma-separated list
	parts := strings.Split(field, ",")
	for _, part := range parts {
		if err := parseFieldPart(part, bits, lo, hi); err != nil {
			return err
		}
	}
	return nil
}

func parseFieldPart(part string, bits []bool, lo, hi int) error {
	// Check for step: */N or N-M/S
	stepStr := ""
	if idx := strings.Index(part, "/"); idx != -1 {
		stepStr = part[idx+1:]
		part = part[:idx]
	}

	var rangeStart, rangeEnd int
	if part == "*" {
		rangeStart = lo
		rangeEnd = hi
	} else if idx := strings.Index(part, "-"); idx != -1 {
		var err error
		rangeStart, err = strconv.Atoi(part[:idx])
		if err != nil {
			return fmt.Errorf("invalid range start %q: %w", part[:idx], err)
		}
		rangeEnd, err = strconv.Atoi(part[idx+1:])
		if err != nil {
			return fmt.Errorf("invalid range end %q: %w", part[idx+1:], err)
		}
	} else {
		val, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid value %q: %w", part, err)
		}
		rangeStart = val
		rangeEnd = val
	}

	if rangeStart < lo || rangeStart > hi {
		return fmt.Errorf("value %d out of range [%d, %d]", rangeStart, lo, hi)
	}
	if rangeEnd < lo || rangeEnd > hi {
		return fmt.Errorf("value %d out of range [%d, %d]", rangeEnd, lo, hi)
	}
	if rangeStart > rangeEnd {
		return fmt.Errorf("range start %d > end %d", rangeStart, rangeEnd)
	}

	step := 1
	if stepStr != "" {
		var err error
		step, err = strconv.Atoi(stepStr)
		if err != nil {
			return fmt.Errorf("invalid step %q: %w", stepStr, err)
		}
		if step < 1 {
			return fmt.Errorf("step must be >= 1, got %d", step)
		}
	}

	for i := rangeStart; i <= rangeEnd; i += step {
		bits[i] = true
	}
	return nil
}

// NextAfter returns the next time matching the expression after the given time.
// Searches up to 4 years ahead before giving up.
func (e *CronExpr) NextAfter(after time.Time) time.Time {
	// Start from the next minute
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.Add(4 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		if !e.month[t.Month()] {
			// Jump to next month
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		// POSIX cron: if both dom and dow are restricted (not *), day matches
		// if EITHER matches. If one is *, only the other is checked.
		domMatch := e.dom[t.Day()]
		dowMatch := e.dow[int(t.Weekday())]
		var dayMatch bool
		if !e.domStar && !e.dowStar {
			dayMatch = domMatch || dowMatch // OR when both restricted
		} else {
			dayMatch = domMatch && dowMatch // AND when either is wildcard
		}
		if !dayMatch {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !e.hour[t.Hour()] {
			// Jump to next hour
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if !e.minute[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	// Should not happen with valid expressions, but return zero time.
	return time.Time{}
}

// ToHuman converts a cron expression string to a human-readable description.
func ToHuman(expr string) string {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return expr
	}

	min, hr, dom, mon, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	// Common patterns
	if min == "*" && hr == "*" && dom == "*" && mon == "*" && dow == "*" {
		return "every minute"
	}
	if strings.HasPrefix(min, "*/") && hr == "*" && dom == "*" && mon == "*" && dow == "*" {
		n := min[2:]
		return "every " + n + " minutes"
	}
	if hr == "*" && dom == "*" && mon == "*" && dow == "*" {
		return "at minute " + min + " of every hour"
	}
	if strings.HasPrefix(hr, "*/") && dom == "*" && mon == "*" && dow == "*" {
		n := hr[2:]
		return "every " + n + " hours at minute " + min
	}

	dayPart := ""
	if dom != "*" && dow != "*" {
		dayPart = " on day " + dom + " and weekday " + dowName(dow)
	} else if dom != "*" {
		dayPart = " on day " + dom
	} else if dow != "*" {
		dayPart = " on " + dowName(dow)
	}

	monPart := ""
	if mon != "*" {
		monPart = " in " + monName(mon)
	}

	timePart := ""
	if hr != "*" && min != "*" {
		timePart = "at " + formatTime(hr, min)
	} else if hr != "*" {
		timePart = "at hour " + hr
	} else {
		timePart = "at minute " + min
	}

	return timePart + dayPart + monPart
}

func formatTime(hr, min string) string {
	h, err := strconv.Atoi(hr)
	if err != nil {
		return hr + ":" + min
	}
	m, err := strconv.Atoi(min)
	if err != nil {
		return hr + ":" + min
	}
	suffix := "AM"
	if h >= 12 {
		suffix = "PM"
	}
	if h > 12 {
		h -= 12
	}
	if h == 0 {
		h = 12
	}
	return fmt.Sprintf("%d:%02d %s", h, m, suffix)
}

func dowName(field string) string {
	names := map[string]string{
		"0": "Sunday", "1": "Monday", "2": "Tuesday", "3": "Wednesday",
		"4": "Thursday", "5": "Friday", "6": "Saturday",
		"1-5": "weekdays", "0,6": "weekends",
	}
	if n, ok := names[field]; ok {
		return n
	}
	return "day-of-week " + field
}

func monName(field string) string {
	names := map[string]string{
		"1": "January", "2": "February", "3": "March", "4": "April",
		"5": "May", "6": "June", "7": "July", "8": "August",
		"9": "September", "10": "October", "11": "November", "12": "December",
	}
	if n, ok := names[field]; ok {
		return n
	}
	return "month " + field
}
