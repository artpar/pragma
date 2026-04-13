package cron

import (
	"fmt"
	"github.com/artpar/gogent/internal/observe"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		observe.GlobalTrace("if: len(fields) != 5")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"cron: expected 5 fields, got %d in %q\", len(fields), expr)")
		return nil, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(fields), expr)
	}

	var c CronExpr
	if err := parseField(fields[0], c.minute[:], 0, 59); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"cron minute: %w\", err)")
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	if err := parseField(fields[1], c.hour[:], 0, 23); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"cron hour: %w\", err)")
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	c.domStar = fields[2] == "*"
	if err := parseField(fields[2], c.dom[:], 1, 31); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"cron day-of-month: %w\", err)")
		return nil, fmt.Errorf("cron day-of-month: %w", err)
	}
	if err := parseField(fields[3], c.month[:], 1, 12); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"cron month: %w\", err)")
		return nil, fmt.Errorf("cron month: %w", err)
	}
	c.dowStar = fields[4] == "*"
	if err := parseField(fields[4], c.dow[:], 0, 6); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"cron day-of-week: %w\", err)")
		return nil, fmt.Errorf("cron day-of-week: %w", err)
	}
	observe.GlobalTrace("return: &c, nil")
	return &c, nil
}

func parseField(field string, bits []bool, lo, hi int) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	parts := strings.Split(field, ",")
	for _, part := range parts {
		observe.GlobalTrace("range parts")
		if err := parseFieldPart(part, bits, lo, hi); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: err")
			return err
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func parseFieldPart(part string, bits []bool, lo, hi int) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	stepStr := ""
	if idx := strings.Index(part, "/"); idx != -1 {
		observe.GlobalTrace("if: idx != -1")
		stepStr = part[idx+1:]
		part = part[:idx]
	}

	var rangeStart, rangeEnd int
	if part == "*" {
		observe.GlobalTrace("if: part == \"*\"")
		rangeStart = lo
		rangeEnd = hi
	} else if idx := strings.Index(part, "-"); idx != -1 {
		observe.GlobalTrace("else-if: idx != -1")
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
		observe.GlobalTrace("if: rangeStart < lo || rangeStart > hi")
		observe.GlobalTrace("return: fmt.Errorf(\"value %d out of range [%d, %d]\", rangeStart, lo, hi)")
		return fmt.Errorf("value %d out of range [%d, %d]", rangeStart, lo, hi)
	}
	if rangeEnd < lo || rangeEnd > hi {
		observe.GlobalTrace("if: rangeEnd < lo || rangeEnd > hi")
		observe.GlobalTrace("return: fmt.Errorf(\"value %d out of range [%d, %d]\", rangeEnd, lo, hi)")
		return fmt.Errorf("value %d out of range [%d, %d]", rangeEnd, lo, hi)
	}
	if rangeStart > rangeEnd {
		observe.GlobalTrace("if: rangeStart > rangeEnd")
		observe.GlobalTrace("return: fmt.Errorf(\"range start %d > end %d\", rangeStart, rangeEnd)")
		return fmt.Errorf("range start %d > end %d", rangeStart, rangeEnd)
	}

	step := 1
	if stepStr != "" {
		observe.GlobalTrace("if: stepStr != \"\"")
		var err error
		step, err = strconv.Atoi(stepStr)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: fmt.Errorf(\"invalid step %q: %w\", stepStr, err)")
			return fmt.Errorf("invalid step %q: %w", stepStr, err)
		}
		if step < 1 {
			observe.GlobalTrace("if: step < 1")
			observe.GlobalTrace("return: fmt.Errorf(\"step must be >= 1, got %d\", step)")
			return fmt.Errorf("step must be >= 1, got %d", step)
		}
	}

	for i := rangeStart; i <= rangeEnd; i += step {
		observe.GlobalTrace("for: i <= rangeEnd")
		bits[i] = true
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// NextAfter returns the next time matching the expression after the given time.
// Searches up to 4 years ahead before giving up.
func (e *CronExpr) NextAfter(after time.Time) time.Time {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.Add(4 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		observe.GlobalTrace("for: t.Before(limit)")
		if !e.month[t.Month()] {
			observe.GlobalTrace("if: !e.month[t.Month()]")

			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		domMatch := e.dom[t.Day()]
		dowMatch := e.dow[int(t.Weekday())]
		var dayMatch bool
		if !e.domStar && !e.dowStar {
			observe.GlobalTrace("if: !e.domStar && !e.dowStar")
			dayMatch = domMatch || dowMatch
		} else {
			observe.GlobalTrace("else: !e.domStar && !e.dowStar")
			dayMatch = domMatch && dowMatch
		}
		if !dayMatch {
			observe.GlobalTrace("if: !dayMatch")
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !e.hour[t.Hour()] {
			observe.GlobalTrace("if: !e.hour[t.Hour()]")

			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if !e.minute[t.Minute()] {
			observe.GlobalTrace("if: !e.minute[t.Minute()]")
			t = t.Add(time.Minute)
			continue
		}
		observe.GlobalTrace("return: t")
		return t
	}
	observe.GlobalTrace("return: time.Time{}")

	return time.Time{}
}

// ToHuman converts a cron expression string to a human-readable description.
func ToHuman(expr string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		observe.GlobalTrace("if: len(fields) != 5")
		observe.GlobalTrace("return: expr")
		return expr
	}

	min, hr, dom, mon, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	if min == "*" && hr == "*" && dom == "*" && mon == "*" && dow == "*" {
		observe.GlobalTrace("if: min == \"*\" && hr == \"*\" && dom == \"*\" && mon == \"*\" && dow == \"*\"")
		observe.GlobalTrace("return: \"every minute\"")
		return "every minute"
	}
	if strings.HasPrefix(min, "*/") && hr == "*" && dom == "*" && mon == "*" && dow == "*" {
		observe.GlobalTrace("if: strings.HasPrefix(min, \"*/\") && hr == \"*\" && dom == \"*\" && mon == \"*\" && dow ...")
		n := min[2:]
		observe.GlobalTrace("return: \"every \" + n + \" minutes\"")
		return "every " + n + " minutes"
	}
	if hr == "*" && dom == "*" && mon == "*" && dow == "*" {
		observe.GlobalTrace("if: hr == \"*\" && dom == \"*\" && mon == \"*\" && dow == \"*\"")
		observe.GlobalTrace("return: \"at minute \" + min + \" of every hour\"")
		return "at minute " + min + " of every hour"
	}
	if strings.HasPrefix(hr, "*/") && dom == "*" && mon == "*" && dow == "*" {
		observe.GlobalTrace("if: strings.HasPrefix(hr, \"*/\") && dom == \"*\" && mon == \"*\" && dow == \"*\"")
		n := hr[2:]
		observe.GlobalTrace("return: \"every \" + n + \" hours at minute \" + min")
		return "every " + n + " hours at minute " + min
	}

	dayPart := ""
	if dom != "*" && dow != "*" {
		observe.GlobalTrace("if: dom != \"*\" && dow != \"*\"")
		dayPart = " on day " + dom + " and weekday " + dowName(dow)
	} else if dom != "*" {
		observe.GlobalTrace("else-if: dom != \"*\"")
		dayPart = " on day " + dom
	} else if dow != "*" {
		dayPart = " on " + dowName(dow)
	}

	monPart := ""
	if mon != "*" {
		observe.GlobalTrace("if: mon != \"*\"")
		monPart = " in " + monName(mon)
	}

	timePart := ""
	if hr != "*" && min != "*" {
		observe.GlobalTrace("if: hr != \"*\" && min != \"*\"")
		timePart = "at " + formatTime(hr, min)
	} else if hr != "*" {
		observe.GlobalTrace("else-if: hr != \"*\"")
		timePart = "at hour " + hr
	} else {
		timePart = "at minute " + min
	}
	observe.GlobalTrace("return: timePart + dayPart + monPart")

	return timePart + dayPart + monPart
}

func formatTime(hr, min string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	h, err := strconv.Atoi(hr)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: hr + \":\" + min")
		return hr + ":" + min
	}
	m, err := strconv.Atoi(min)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: hr + \":\" + min")
		return hr + ":" + min
	}
	suffix := "AM"
	if h >= 12 {
		observe.GlobalTrace("if: h >= 12")
		suffix = "PM"
	}
	if h > 12 {
		observe.GlobalTrace("if: h > 12")
		h -= 12
	}
	if h == 0 {
		observe.GlobalTrace("if: h == 0")
		h = 12
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"%d:%02d %s\", h, m, suffix)")
	return fmt.Sprintf("%d:%02d %s", h, m, suffix)
}

func dowName(field string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	names := map[string]string{
		"0": "Sunday", "1": "Monday", "2": "Tuesday", "3": "Wednesday",
		"4": "Thursday", "5": "Friday", "6": "Saturday",
		"1-5": "weekdays", "0,6": "weekends",
	}
	if n, ok := names[field]; ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: n")
		return n
	}
	observe.GlobalTrace("return: \"day-of-week \" + field")
	return "day-of-week " + field
}

func monName(field string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	names := map[string]string{
		"1": "January", "2": "February", "3": "March", "4": "April",
		"5": "May", "6": "June", "7": "July", "8": "August",
		"9": "September", "10": "October", "11": "November", "12": "December",
	}
	if n, ok := names[field]; ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: n")
		return n
	}
	observe.GlobalTrace("return: \"month \" + field")
	return "month " + field
}
