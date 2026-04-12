package middleware

import (
	"net/url"
	"strings"
)

const redactedLogValue = "[REDACTED]"

func SanitizeLogQuery(rawQuery string) string {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return ""
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return redactedLogValue
	}

	for key, entries := range values {
		if !isSensitiveLogKey(key) {
			continue
		}
		for i := range entries {
			entries[i] = redactedLogValue
		}
		values[key] = entries
	}

	return values.Encode()
}

func SanitizeLogReferer(rawReferer string) string {
	rawReferer = strings.TrimSpace(rawReferer)
	if rawReferer == "" {
		return ""
	}

	parsed, err := url.Parse(rawReferer)
	if err != nil {
		return redactedLogValue
	}

	if parsed.User != nil {
		username := parsed.User.Username()
		if username != "" {
			parsed.User = url.UserPassword(username, redactedLogValue)
		} else {
			parsed.User = url.User(redactedLogValue)
		}
	}
	parsed.RawQuery = SanitizeLogQuery(parsed.RawQuery)
	if strings.Contains(parsed.Fragment, "=") {
		parsed.Fragment = SanitizeLogQuery(parsed.Fragment)
	}

	return parsed.String()
}

func isSensitiveLogKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}

	sensitiveFragments := []string{
		"password",
		"passwd",
		"secret",
		"token",
		"authorization",
		"api_key",
		"apikey",
	}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}
