package env

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var interpolationPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

type Loader struct{}

func NewLoader() Loader {
	return Loader{}
}

func (Loader) Load(path string) (map[string]string, error) {
	resolved := strings.TrimSpace(path)
	if resolved == "" {
		resolved = ".env"
	}

	if _, err := os.Stat(resolved); err != nil {
		if path == "" && os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read env file %q: %w", resolved, err)
	}

	file, err := os.Open(filepath.Clean(resolved))
	if err != nil {
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		split := strings.SplitN(line, "=", 2)
		if len(split) != 2 {
			continue
		}
		key := strings.TrimSpace(split[0])
		rawValue := strings.TrimSpace(split[1])
		values[key] = stripQuotes(rawValue)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan env file: %w", err)
	}

	for key := range values {
		values[key] = interpolate(values, values[key], 0)
	}

	return values, nil
}

func stripQuotes(v string) string {
	if len(v) < 2 {
		return v
	}
	if (strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"")) || (strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'")) {
		return v[1 : len(v)-1]
	}
	return v
}

func interpolate(values map[string]string, input string, depth int) string {
	if depth > 8 {
		return input
	}

	output := interpolationPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := interpolationPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		key := parts[1]
		if value, ok := values[key]; ok {
			return interpolate(values, value, depth+1)
		}
		if value := os.Getenv(key); value != "" {
			return value
		}
		return ""
	})
	return output
}
