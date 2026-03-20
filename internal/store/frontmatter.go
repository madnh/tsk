package store

import (
	"fmt"
	"strings"

	"github.com/user/tsk/internal/model"
)

// ParseFrontmatter parses YAML frontmatter from markdown content.
// Uses line-by-line parsing (NOT yaml.v3) to match Node.js format exactly.
// Arrays are formatted as [item1, item2] on a single line.
func ParseFrontmatter(content string) (map[string]interface{}, string) {
	if !strings.HasPrefix(content, "---\n") {
		return map[string]interface{}{}, content
	}

	rest := content[4:] // skip "---\n"
	endIdx := strings.Index(rest, "\n---\n")
	if endIdx == -1 {
		return map[string]interface{}{}, content
	}

	metaStr := rest[:endIdx]
	body := rest[endIdx+5:] // skip "\n---\n"

	meta := map[string]interface{}{}
	for _, line := range strings.Split(metaStr, "\n") {
		idx := strings.Index(line, ":")
		if idx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
			inner := val[1 : len(val)-1]
			if inner == "" {
				meta[key] = []string{}
			} else {
				parts := strings.Split(inner, ",")
				arr := make([]string, 0, len(parts))
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						arr = append(arr, p)
					}
				}
				meta[key] = arr
			}
		} else {
			meta[key] = val
		}
	}

	return meta, body
}

// SerializeFrontmatter serializes key-value pairs and body into frontmatter format.
func SerializeFrontmatter(kvs []model.KV, body string) string {
	var lines []string
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case []string:
			lines = append(lines, fmt.Sprintf("%s: [%s]", kv.Key, strings.Join(v, ", ")))
		case string:
			lines = append(lines, fmt.Sprintf("%s: %s", kv.Key, v))
		default:
			lines = append(lines, fmt.Sprintf("%s: %v", kv.Key, v))
		}
	}
	return "---\n" + strings.Join(lines, "\n") + "\n---\n" + body
}

// GetString gets a string value from meta map
func GetString(meta map[string]interface{}, key string) string {
	v, ok := meta[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// GetStringSlice gets a string slice from meta map
func GetStringSlice(meta map[string]interface{}, key string) []string {
	v, ok := meta[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case string:
		if val == "" {
			return nil
		}
		return []string{val}
	default:
		return nil
	}
}
