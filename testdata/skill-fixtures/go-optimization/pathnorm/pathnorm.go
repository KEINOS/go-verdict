package pathnorm

import "strings"

// NormalizeSlashPath collapses repeated slash separators and removes trailing
// slashes, except that the root path remains "/".
func NormalizeSlashPath(path string) string {
	return normalizeSlashPathEnhanced(path)
}

func normalizeSlashPathOriginal(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return "/"
	}
	return strings.Join(out, "/")
}

func normalizeSlashPathEnhanced(path string) string {
	if path == "" {
		return ""
	}

	var sb strings.Builder
	sb.Grow(len(path))

	segmentStart := -1
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if segmentStart != -1 {
				if sb.Len() > 0 {
					sb.WriteByte('/')
				}
				sb.WriteString(path[segmentStart:i])
				segmentStart = -1
			}
			continue
		}
		if segmentStart == -1 {
			segmentStart = i
		}
	}

	if sb.Len() == 0 {
		return "/"
	}
	return sb.String()
}
