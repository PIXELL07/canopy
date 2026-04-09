// Package validate provides reusable input validation helpers.
// Every public function returns a slice of apierr.FieldErr so callers
// can accumulate failures and return them all at once instead of stopping
// at the first bad field.
package validate

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/pixell07/canopy/internal/apierr"
)

// fails if the string is empty or whitespace-only.
func Required(field, value string) *apierr.FieldErr {
	if strings.TrimSpace(value) == "" {
		e := apierr.Field(field, "is required")
		return &e
	}
	return nil
}

// fails if the string has fewer than min UTF-8 characters.
func MinLen(field, value string, min int) *apierr.FieldErr {
	if utf8.RuneCountInString(value) < min {
		e := apierr.Field(field, "must be at least "+itoa(min)+" characters")
		return &e
	}
	return nil
}

// fails if the string exceeds max UTF-8 characters.
func MaxLen(field, value string, max int) *apierr.FieldErr {
	if utf8.RuneCountInString(value) > max {
		e := apierr.Field(field, "must be at most "+itoa(max)+" characters")
		return &e
	}
	return nil
}

// Email does a lightweight format check (not RFC-exhaustive).
func Email(field, value string) *apierr.FieldErr {
	parts := strings.Split(value, "@")
	if len(parts) != 2 || parts[0] == "" || !strings.Contains(parts[1], ".") {
		e := apierr.Field(field, "must be a valid email address")
		return &e
	}
	return nil
}

// fails if value is outside [min, max] inclusive.
func InRange(field string, value, min, max int) *apierr.FieldErr {
	if value < min || value > max {
		e := apierr.Field(field, "must be between "+itoa(min)+" and "+itoa(max))
		return &e
	}
	return nil
}

// fails if the integer is not > 0.
func Positive(field string, value int64) *apierr.FieldErr {
	if value <= 0 {
		e := apierr.Field(field, "must be a positive integer")
		return &e
	}
	return nil
}

// fails if value is outside [min, max] inclusive.
func FloatRange(field string, value, min, max float64) *apierr.FieldErr {
	if value < min || value > max {
		e := apierr.Field(field, "must be between "+fmtFloat(min)+" and "+fmtFloat(max))
		return &e
	}
	return nil
}

// fails if the string is not a parseable absolute URL with http/https scheme.
func ValidURL(field, value string) *apierr.FieldErr {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		e := apierr.Field(field, "must be a valid http or https URL")
		return &e
	}
	return nil
}

// Collect gathers non-nil FieldErrs into a slice.
// Usage: errs := validate.Collect(validate.Required("name", body.Name), validate.Email("email", body.Email))
func Collect(errs ...*apierr.FieldErr) []apierr.FieldErr {
	var out []apierr.FieldErr
	for _, e := range errs {
		if e != nil {
			out = append(out, *e)
		}
	}
	return out
}

// helpers

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func fmtFloat(f float64) string {
	// Simple two-decimal formatting without fmt import
	s := itoa(int(f * 100))
	for len(s) < 3 {
		s = "0" + s
	}
	return s[:len(s)-2] + "." + s[len(s)-2:]
}
