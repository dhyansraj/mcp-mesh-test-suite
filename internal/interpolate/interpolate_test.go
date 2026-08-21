package interpolate

import "testing"

func newTestContext() *Context {
	ctx := NewContext()
	ctx.Captured["token"] = "s3cret"
	ctx.Captured["key"] = "OPENAI_API_KEY"
	return ctx
}

func TestInterpolateMapStringMapValues(t *testing.T) {
	ctx := newTestContext()

	out, err := InterpolateMap(map[string]any{
		"headers": map[string]string{"Authorization": "Bearer ${captured.token}"},
	}, ctx)
	if err != nil {
		t.Fatalf("InterpolateMap() error = %v", err)
	}

	// Normalized to map[string]any so handlers only assert on one shape.
	headers, ok := out["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers = %#v, want map[string]any", out["headers"])
	}
	if headers["Authorization"] != "Bearer s3cret" {
		t.Errorf("Authorization = %#v, want %q", headers["Authorization"], "Bearer s3cret")
	}
}

func TestInterpolateMapStringSliceValues(t *testing.T) {
	ctx := newTestContext()

	out, err := InterpolateMap(map[string]any{"keys": []string{"${captured.key}", "DATABASE_URL"}}, ctx)
	if err != nil {
		t.Fatalf("InterpolateMap() error = %v", err)
	}

	keys, ok := out["keys"].([]any)
	if !ok || len(keys) != 2 {
		t.Fatalf("keys = %#v, want []any of len 2", out["keys"])
	}
	if keys[0] != "OPENAI_API_KEY" || keys[1] != "DATABASE_URL" {
		t.Errorf("keys = %v, want [OPENAI_API_KEY DATABASE_URL]", keys)
	}
}

func TestInterpolateValueLeavesOtherTypes(t *testing.T) {
	ctx := newTestContext()

	for _, v := range []any{42, true, 2.5, nil} {
		got, err := InterpolateValue(v, ctx)
		if err != nil {
			t.Fatalf("InterpolateValue(%#v) error = %v", v, err)
		}
		if got != v {
			t.Errorf("InterpolateValue(%#v) = %#v, want unchanged", v, got)
		}
	}
}

func TestInterpolateMapNested(t *testing.T) {
	ctx := newTestContext()

	out, err := InterpolateMap(map[string]any{
		"body": map[string]any{
			"auth": map[string]string{"token": "${captured.token}"},
			"list": []any{"${captured.key}"},
		},
	}, ctx)
	if err != nil {
		t.Fatalf("InterpolateMap() error = %v", err)
	}

	body := out["body"].(map[string]any)
	auth, ok := body["auth"].(map[string]any)
	if !ok {
		t.Fatalf("body.auth = %#v, want map[string]any", body["auth"])
	}
	if auth["token"] != "s3cret" {
		t.Errorf("body.auth.token = %#v, want s3cret", auth["token"])
	}
	list := body["list"].([]any)
	if list[0] != "OPENAI_API_KEY" {
		t.Errorf("body.list[0] = %#v, want OPENAI_API_KEY", list[0])
	}
}
