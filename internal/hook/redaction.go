package hook

import (
	"encoding/json"
	"regexp"
	"strings"
)

const redactedValue = "<redacted>"

var defaultRedactionKeys = []string{
	"token",
	"password",
	"passwd",
	"secret",
	"credential",
	"credentials",
	"auth",
	"authorization",
	"api_key",
	"apikey",
	"access_key",
	"private_key",
	"registry_auth",
}

// RedactionConfig controls best-effort trace redaction.
type RedactionConfig struct {
	Enabled bool
	Keys    []string
}

func defaultRedactionConfig(extraKeys []string) RedactionConfig {
	keys := append([]string{}, defaultRedactionKeys...)
	keys = append(keys, extraKeys...)
	return RedactionConfig{Enabled: true, Keys: normalizeRedactionKeys(keys)}
}

func normalizeRedactionKeys(keys []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		lower := strings.ToLower(key)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		out = append(out, lower)
	}
	return out
}

func (c RedactionConfig) keyLooksSensitive(key string) bool {
	key = strings.ToLower(key)
	normalizedKey := normalizeKeyName(key)
	for _, pattern := range c.Keys {
		pattern = strings.ToLower(pattern)
		if strings.Contains(key, pattern) || strings.Contains(normalizedKey, normalizeKeyName(pattern)) {
			return true
		}
	}
	return false
}

func redactEnv(env []string, cfg RedactionConfig) []string {
	if !cfg.Enabled || len(env) == 0 {
		return env
	}
	out := append([]string{}, env...)
	for i, item := range out {
		name, _, ok := strings.Cut(item, "=")
		if ok && cfg.keyLooksSensitive(name) {
			out[i] = name + "=" + redactedValue
		}
	}
	return out
}

func redactStringSlice(values []string, cfg RedactionConfig) []string {
	if !cfg.Enabled || len(values) == 0 {
		return values
	}
	out := append([]string{}, values...)
	for i, value := range out {
		out[i] = redactText(value, cfg)
	}
	return out
}

func redactJSON(raw json.RawMessage, cfg RedactionConfig) json.RawMessage {
	if !cfg.Enabled || len(raw) == 0 {
		return raw
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	value = redactJSONValue(value, cfg, "")
	data, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return data
}

func redactJSONValue(value any, cfg RedactionConfig, key string) any {
	if key != "" && cfg.keyLooksSensitive(key) {
		return redactedValue
	}

	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for childKey, childValue := range v {
			out[childKey] = redactJSONValue(childValue, cfg, childKey)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, childValue := range v {
			out[i] = redactJSONValue(childValue, cfg, "")
		}
		return out
	default:
		return value
	}
}

func redactText(s string, cfg RedactionConfig) string {
	if !cfg.Enabled || s == "" {
		return s
	}
	for _, key := range cfg.Keys {
		if key == "" {
			continue
		}
		pattern := `(?i)(\b` + regexp.QuoteMeta(key) + `\b\s*[:=]\s*)("[^"]*"|'[^']*'|[^\n,;]+)`
		re := regexp.MustCompile(pattern)
		s = re.ReplaceAllString(s, `${1}`+redactedValue)
	}
	return s
}

func normalizeKeyName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
