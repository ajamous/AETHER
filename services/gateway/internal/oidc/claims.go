package oidc

import "time"

// audienceMatches reports whether want is in the aud claim. Per RFC
// 7519 §4.1.3 aud may be a single string or an array of strings;
// both are accepted.
func audienceMatches(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return v == want
	case []any:
		for _, a := range v {
			if s, _ := a.(string); s == want {
				return true
			}
		}
	}
	return false
}

// audienceSlice returns the aud claim as a string slice. Mirrors
// audienceMatches's input handling.
func audienceSlice(aud any) []string {
	switch v := aud.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, a := range v {
			if s, ok := a.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// claimAsTime decodes an exp/nbf/iat claim. JWT defines these as
// NumericDate (seconds since epoch). encoding/json hands them back
// as float64; tests sometimes pass int directly. Both shapes work.
func claimAsTime(c any) (time.Time, bool) {
	switch v := c.(type) {
	case float64:
		return time.Unix(int64(v), 0), true
	case int64:
		return time.Unix(v, 0), true
	case int:
		return time.Unix(int64(v), 0), true
	case json_Number:
		i, err := v.Int64()
		if err != nil {
			return time.Time{}, false
		}
		return time.Unix(i, 0), true
	}
	return time.Time{}, false
}

// json_Number aliases encoding/json.Number without importing it
// into oidc.go. We accept it here for callers that decode with
// UseNumber. The alias keeps the type assertion small.
type json_Number interface {
	Int64() (int64, error)
}
