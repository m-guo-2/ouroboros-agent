package oss

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// GenerateObjectKey creates a stable object key under the configured prefix.
func GenerateObjectKey(prefix, fileName string) (string, error) {
	return generateObjectKey(prefix, fileName, time.Now().UTC(), rand.Reader)
}

func generateObjectKey(prefix, fileName string, now time.Time, random io.Reader) (string, error) {
	baseName := sanitizeFileName(fileName)
	ext := path.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)
	if stem == "" {
		stem = "object"
	}

	suffix, err := randomHex(random, 6)
	if err != nil {
		return "", wrapOperationError("generate_object_key", "", ErrInternal, err)
	}

	datePrefix := now.Format("2006/01/02")
	filePart := fmt.Sprintf("%s-%s%s", stem, suffix, ext)
	fullPrefix := normalizeKeyPrefix(prefix)
	if fullPrefix == "" {
		return path.Join(datePrefix, filePart), nil
	}
	return path.Join(fullPrefix, datePrefix, filePart), nil
}

func normalizeKeyPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return ""
	}
	parts := strings.FieldsFunc(prefix, func(r rune) bool {
		return r == '/'
	})
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = sanitizeSegment(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, "/")
}

func sanitizeFileName(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	fileName = path.Base(strings.ReplaceAll(fileName, "\\", "/"))
	if fileName == "" || fileName == "." || fileName == "/" {
		return "object"
	}
	ext := path.Ext(fileName)
	stem := strings.TrimSuffix(fileName, ext)
	stem = sanitizeSegment(stem)
	if stem == "" {
		stem = "object"
	}
	ext = sanitizeExtension(ext)
	return stem + ext
}

func sanitizeSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	lastDash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-.")
}

func sanitizeExtension(ext string) string {
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	var b strings.Builder
	b.Grow(len(ext))
	b.WriteByte('.')
	for _, r := range strings.ToLower(strings.TrimPrefix(ext, ".")) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 1 {
		return ""
	}
	return b.String()
}

func randomHex(reader io.Reader, bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
