package handlers

import (
	"strings"
	"testing"
	"time"
)

// TestDurationField pins the semantics every handler now shares: YAML's int,
// float64 and string forms all resolve, a bare number means seconds, an absent
// key means the default, and anything else is an error rather than a silent
// fall back to the default.
func TestDurationField(t *testing.T) {
	const def = 42 * time.Second

	tests := []struct {
		name    string
		step    map[string]any
		want    time.Duration
		wantErr string
	}{
		{name: "absent uses default", step: map[string]any{}, want: def},
		{name: "nil uses default", step: map[string]any{"timeout": nil}, want: def},
		{name: "bare int is seconds", step: map[string]any{"timeout": 300}, want: 300 * time.Second},
		{name: "int64 is seconds", step: map[string]any{"timeout": int64(90)}, want: 90 * time.Second},
		{name: "float is fractional seconds", step: map[string]any{"timeout": 2.5}, want: 2500 * time.Millisecond},
		{name: "duration value passes through", step: map[string]any{"timeout": 7 * time.Minute}, want: 7 * time.Minute},
		{name: "duration string minutes", step: map[string]any{"timeout": "5m"}, want: 5 * time.Minute},
		{name: "duration string seconds", step: map[string]any{"timeout": "30s"}, want: 30 * time.Second},
		{name: "duration string compound", step: map[string]any{"timeout": "1m30s"}, want: 90 * time.Second},
		{name: "duration string sub-second", step: map[string]any{"timeout": "250ms"}, want: 250 * time.Millisecond},
		{name: "numeric string is seconds", step: map[string]any{"timeout": "300"}, want: 300 * time.Second},
		{name: "numeric string with spaces", step: map[string]any{"timeout": "  300  "}, want: 300 * time.Second},
		{name: "fractional numeric string", step: map[string]any{"timeout": "0.5"}, want: 500 * time.Millisecond},

		{name: "typo is an error", step: map[string]any{"timeout": "fivem"}, wantErr: `invalid timeout: "fivem" is neither`},
		{name: "empty string is an error", step: map[string]any{"timeout": ""}, wantErr: "invalid timeout: value is empty"},
		{name: "blank string is an error", step: map[string]any{"timeout": "   "}, wantErr: "invalid timeout: value is empty"},
		{name: "wrong type is an error", step: map[string]any{"timeout": true}, wantErr: "invalid timeout: true (bool) is neither"},
		{name: "list is an error", step: map[string]any{"timeout": []any{1}}, wantErr: "invalid timeout:"},
		{name: "zero is an error", step: map[string]any{"timeout": 0}, wantErr: "invalid timeout: must be positive"},
		{name: "negative is an error", step: map[string]any{"timeout": -5}, wantErr: "invalid timeout: must be positive"},
		{name: "negative duration string is an error", step: map[string]any{"timeout": "-5m"}, wantErr: "invalid timeout: must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := durationField(tt.step, "timeout", def)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("durationField() = %v, want error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("durationField() error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("durationField() error = %v, want %v", err, tt.want)
			}
			if got != tt.want {
				t.Errorf("durationField() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDurationFieldNamesTheField makes sure the error points at the option the
// author actually typed, since a step can carry several duration fields.
func TestDurationFieldNamesTheField(t *testing.T) {
	for _, key := range []string{"timeout", "interval", "seconds"} {
		t.Run(key, func(t *testing.T) {
			_, err := durationField(map[string]any{key: "nope"}, key, time.Second)
			if err == nil {
				t.Fatal("durationField() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), "invalid "+key) {
				t.Errorf("durationField() error = %q, want it to name %q", err, key)
			}
		})
	}
}
