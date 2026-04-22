package vault

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"obsidian-harness/internal/model"
)

var (
	wikiRefPattern = regexp.MustCompile(`!?\[\[([^\]]+)\]\]`)
	mdRefPattern   = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
)

func ExtractAttachmentRefs(markdown string) []model.AttachmentRef {
	refs := make([]model.AttachmentRef, 0)
	seen := make(map[string]int)

	appendRef := func(ref model.AttachmentRef) {
		if ref.Path == "" || ref.Kind == "" {
			return
		}
		key := ref.Path + "|" + ref.Kind
		if index, ok := seen[key]; ok {
			if ref.Embed {
				refs[index].Embed = true
			}
			return
		}
		seen[key] = len(refs)
		refs = append(refs, ref)
	}

	for _, match := range wikiRefPattern.FindAllStringSubmatchIndex(markdown, -1) {
		fullMatch := markdown[match[0]:match[1]]
		rawTarget := markdown[match[2]:match[3]]
		if ref, ok := buildAttachmentRef(rawTarget, strings.HasPrefix(fullMatch, "!"), true); ok {
			appendRef(ref)
		}
	}

	for _, match := range mdRefPattern.FindAllStringSubmatchIndex(markdown, -1) {
		fullMatch := markdown[match[0]:match[1]]
		rawTarget := markdown[match[2]:match[3]]
		if ref, ok := buildAttachmentRef(rawTarget, strings.HasPrefix(fullMatch, "!"), false); ok {
			appendRef(ref)
		}
	}

	return refs
}

func buildAttachmentRef(rawTarget string, embed bool, wiki bool) (model.AttachmentRef, bool) {
	rawTarget = strings.TrimSpace(rawTarget)
	if rawTarget == "" {
		return model.AttachmentRef{}, false
	}

	normalized := rawTarget
	if wiki {
		normalized, _, _ = strings.Cut(normalized, "|")
	} else {
		normalized = stripMarkdownDestination(normalized)
	}
	normalized = strings.TrimSpace(normalized)
	normalized = strings.TrimPrefix(normalized, "<")
	normalized = strings.TrimSuffix(normalized, ">")
	if normalized == "" || isExternalReference(normalized) {
		return model.AttachmentRef{}, false
	}

	normalized, _, _ = strings.Cut(normalized, "#")
	normalized = filepath.ToSlash(filepath.Clean(strings.TrimSpace(normalized)))
	kind := detectAttachmentKind(normalized)
	if kind == "" {
		return model.AttachmentRef{}, false
	}

	return model.AttachmentRef{
		Raw:   rawTarget,
		Path:  normalized,
		Kind:  kind,
		Embed: embed,
	}, true
}

func stripMarkdownDestination(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		return strings.TrimSpace(value[1 : len(value)-1])
	}

	inQuote := rune(0)
	for i, r := range value {
		switch {
		case r == '"' || r == '\'':
			if inQuote == 0 {
				inQuote = r
			} else if inQuote == r {
				inQuote = 0
			}
		case unicode.IsSpace(r) && inQuote == 0:
			rest := strings.TrimSpace(value[i:])
			if strings.HasPrefix(rest, "\"") || strings.HasPrefix(rest, "'") {
				return strings.TrimSpace(value[:i])
			}
		}
	}
	return value
}

func isExternalReference(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "mailto:")
}

func detectAttachmentKind(value string) string {
	ext := strings.ToLower(filepath.Ext(value))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".avif":
		return "image"
	case ".pdf":
		return "pdf"
	case ".mp3", ".wav", ".m4a", ".ogg", ".flac":
		return "audio"
	case ".mp4", ".mov", ".avi", ".webm", ".mkv", ".m4v":
		return "video"
	case ".md", ".markdown", "":
		return ""
	default:
		return "file"
	}
}
