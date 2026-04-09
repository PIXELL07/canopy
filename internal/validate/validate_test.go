package validate_test

import (
	"testing"

	"github.com/pixell07/canopy/internal/validate"
)

func TestRequired(t *testing.T) {
	if e := validate.Required("field", "value"); e != nil {
		t.Error("non-empty string should pass Required")
	}
	if e := validate.Required("field", ""); e == nil {
		t.Error("empty string should fail Required")
	}
	if e := validate.Required("field", "   "); e == nil {
		t.Error("whitespace-only should fail Required")
	}
}

func TestMinLen(t *testing.T) {
	if e := validate.MinLen("p", "hello", 5); e != nil {
		t.Error("exact min length should pass")
	}
	if e := validate.MinLen("p", "hi", 5); e == nil {
		t.Error("below min length should fail")
	}
}

func TestMaxLen(t *testing.T) {
	if e := validate.MaxLen("f", "hello", 10); e != nil {
		t.Error("within max should pass")
	}
	if e := validate.MaxLen("f", "hello world", 5); e == nil {
		t.Error("exceeds max should fail")
	}
}

func TestEmail(t *testing.T) {
	valid := []string{"user@example.com", "a@b.co", "x+y@domain.org"}
	for _, e := range valid {
		if err := validate.Email("email", e); err != nil {
			t.Errorf("valid email %q should pass, got: %v", e, err)
		}
	}
	invalid := []string{"", "notanemail", "missing@tld", "@nodomain.com"}
	for _, e := range invalid {
		if err := validate.Email("email", e); err == nil {
			t.Errorf("invalid email %q should fail", e)
		}
	}
}

func TestInRange(t *testing.T) {
	if e := validate.InRange("f", 5, 1, 10); e != nil {
		t.Error("value in range should pass")
	}
	if e := validate.InRange("f", 1, 1, 10); e != nil {
		t.Error("lower bound inclusive should pass")
	}
	if e := validate.InRange("f", 10, 1, 10); e != nil {
		t.Error("upper bound inclusive should pass")
	}
	if e := validate.InRange("f", 0, 1, 10); e == nil {
		t.Error("below range should fail")
	}
	if e := validate.InRange("f", 11, 1, 10); e == nil {
		t.Error("above range should fail")
	}
}

func TestFloatRange(t *testing.T) {
	if e := validate.FloatRange("f", 0.05, 0.0, 1.0); e != nil {
		t.Error("valid float should pass")
	}
	if e := validate.FloatRange("f", 1.5, 0.0, 1.0); e == nil {
		t.Error("float above max should fail")
	}
	if e := validate.FloatRange("f", -0.1, 0.0, 1.0); e == nil {
		t.Error("negative float should fail")
	}
}

func TestValidURL(t *testing.T) {
	valid := []string{"https://example.com", "http://localhost:8080/webhook", "https://hooks.slack.com/T00/B00/xxx"}
	for _, u := range valid {
		if e := validate.ValidURL("url", u); e != nil {
			t.Errorf("valid URL %q should pass, got: %v", u, e)
		}
	}
	invalid := []string{"", "not-a-url", "ftp://wrong-scheme.com", "//no-scheme.com"}
	for _, u := range invalid {
		if e := validate.ValidURL("url", u); e == nil {
			t.Errorf("invalid URL %q should fail", u)
		}
	}
}

func TestCollect(t *testing.T) {
	errs := validate.Collect(
		validate.Required("a", "ok"),   // passes — nil
		validate.Required("b", ""),     // fails — non-nil
		validate.MinLen("c", "hi", 10), // fails — non-nil
	)
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}
	if errs[0].Field != "b" {
		t.Errorf("expected first error on field 'b', got '%s'", errs[0].Field)
	}
}
