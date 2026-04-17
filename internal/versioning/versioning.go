package versioning

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"sloth/internal/storage"
)

func BuildVersionedPrefix(basePath string, serviceID string) string {
	base := strings.Trim(basePath, "/")
	return path.Join(base, serviceID)
}

func NextVersionID(objects []storage.ObjectInfo, servicePrefix string) string {
	maxVersion := 0
	normalized := strings.Trim(servicePrefix, "/")
	if normalized != "" {
		normalized += "/"
	}

	for _, object := range objects {
		key := strings.Trim(object.Key, "/")
		if !strings.HasPrefix(key, normalized) {
			continue
		}
		remainder := strings.TrimPrefix(key, normalized)
		if remainder == "" {
			continue
		}
		split := strings.SplitN(remainder, "/", 2)
		candidate := split[0]
		number, err := strconv.Atoi(candidate)
		if err != nil {
			continue
		}
		if number > maxVersion {
			maxVersion = number
		}
	}

	return strconv.Itoa(maxVersion + 1)
}

func ExtractVersionFromKey(key string, servicePrefix string) string {
	normalizedPrefix := strings.Trim(servicePrefix, "/")
	normalizedKey := strings.Trim(key, "/")

	if normalizedPrefix != "" {
		normalizedPrefix += "/"
	}

	if !strings.HasPrefix(normalizedKey, normalizedPrefix) {
		return ""
	}
	remainder := strings.TrimPrefix(normalizedKey, normalizedPrefix)
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return ""
	}
	return parts[0]
}

func SortByLastModifiedDesc(objects []storage.ObjectInfo) {
	sort.Slice(objects, func(i int, j int) bool {
		return objects[i].LastModified.After(objects[j].LastModified)
	})
}

func SelectLatestVersion(objects []storage.ObjectInfo, servicePrefix string) (string, error) {
	versions := map[string]struct{}{}
	for _, object := range objects {
		version := ExtractVersionFromKey(object.Key, servicePrefix)
		if version == "" {
			continue
		}
		versions[version] = struct{}{}
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no backup versions found")
	}

	highest := -1
	for version := range versions {
		n, err := strconv.Atoi(version)
		if err != nil {
			continue
		}
		if n > highest {
			highest = n
		}
	}

	if highest < 0 {
		return "", fmt.Errorf("no numeric backup versions found")
	}

	return strconv.Itoa(highest), nil
}
