// Package textutil provides UTF-8 safe helpers for bounding the size of
// captured command output before it is stored, transmitted, or folded into an
// error message.
package textutil

import (
	"fmt"
	"strings"
)

const (
	// MaxStepOutput bounds a single stdout/stderr blob reported for one step.
	// step_results.stdout/stderr are unconstrained SQLite TEXT columns, so a
	// chatty step (build logs, verbose test output) would otherwise write an
	// unbounded blob into the database on every run.
	MaxStepOutput = 64 * 1024

	// MaxErrorDetail bounds how much output is folded into an error message.
	MaxErrorDetail = 2000

	// headFraction is the share of the budget kept from the start of the output;
	// the remainder is kept from the end, where failures usually surface.
	headFraction = 4
)

// Truncate caps s at max bytes, keeping the head and appending a marker when it
// was cut. Intended for short diagnostic details, not for full output.
func Truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.ToValidUTF8(s[:max], "") + "... (truncated)"
}

// TruncateOutput caps s at roughly max bytes, keeping both the head and the
// tail with a marker in between. The tail is kept because the end of a command's
// output is where the failure detail lives; a head-only cut would throw it away.
func TruncateOutput(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	head := max / headFraction
	tail := max - head
	// ToValidUTF8 drops the partial rune left at each cut point.
	return fmt.Sprintf("%s\n... (%d bytes truncated) ...\n%s",
		strings.ToValidUTF8(s[:head], ""),
		len(s)-head-tail,
		strings.ToValidUTF8(s[len(s)-tail:], ""))
}
