package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/vault"
)

func (h *Harness) ManagedStatus() (model.ManagedStatusView, error) {
	coreDocs := []model.ManagedCoreStatus{
		h.coreDocStatus("system", h.cfg.Vault.ManagedCore.SystemDoc),
		h.coreDocStatus("progress", h.cfg.Vault.ManagedCore.ProgressIndex),
		h.coreDocStatus("persona", h.cfg.Vault.ManagedCore.Persona),
	}

	ready := true
	for _, doc := range coreDocs {
		if !doc.Exists {
			ready = false
			break
		}
	}

	view := model.ManagedStatusView{
		Ready:     ready,
		WorkDir:   h.cfg.Paths.WorkDir,
		VaultRoot: h.cfg.Paths.VaultRoot,
		CoreDocs:  coreDocs,
		Health:    h.health.Snapshot(),
	}

	h.recordReadAudit("managed_status", "vault", map[string]string{
		"ready": fmt.Sprintf("%t", view.Ready),
	})
	return view, nil
}

func (h *Harness) SystemDocGet(name string) (model.VaultDocument, error) {
	relPath, docClass, err := h.resolveSystemDoc(name)
	if err != nil {
		return model.VaultDocument{}, err
	}
	doc, err := h.readDocument(relPath, docClass)
	if err != nil {
		return model.VaultDocument{}, err
	}

	h.recordReadAudit("system_doc_get", relPath, map[string]string{
		"name": name,
	})
	return doc, nil
}

func (h *Harness) VaultRead(relPath string) (model.VaultDocument, error) {
	if !isMarkdownDocPath(relPath) {
		return model.VaultDocument{}, fmt.Errorf("vault_read only supports markdown documents: %s", cleanRelPath(relPath))
	}
	classification := h.classifier.Classify(relPath)
	doc, err := h.readDocument(relPath, classification.Class)
	if err != nil {
		return model.VaultDocument{}, err
	}

	h.recordReadAudit("vault_read", doc.Path, map[string]string{
		"doc_class": string(doc.DocClass),
	})
	return doc, nil
}

func (h *Harness) VaultList(relDir string) ([]model.VaultEntry, error) {
	list, err := vault.ListEntries(h.cfg.Paths.VaultRoot, relDir)
	if err != nil {
		return nil, err
	}

	entries := make([]model.VaultEntry, 0, len(list))
	for _, entry := range list {
		docClass := model.DocClassUnknown
		if !entry.IsDir {
			docClass = h.classifier.Classify(entry.Path).Class
		}
		entries = append(entries, model.VaultEntry{
			Path:     entry.Path,
			Name:     entry.Name,
			Kind:     entryKind(entry.IsDir),
			DocClass: docClass,
		})
	}

	h.recordReadAudit("vault_list", normalizeAuditTarget(relDir), map[string]string{
		"count": fmt.Sprintf("%d", len(entries)),
	})
	return entries, nil
}

func (h *Harness) VaultSearchText(query string, relDir string, limit int) ([]model.SearchHit, error) {
	hits, err := vault.SearchText(h.cfg.Paths.VaultRoot, relDir, query, limit)
	if err != nil {
		return nil, err
	}

	out := make([]model.SearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, model.SearchHit{
			Path:     hit.Path,
			Line:     hit.Line,
			Preview:  hit.Preview,
			DocClass: h.classifier.Classify(hit.Path).Class,
		})
	}

	h.recordReadAudit("vault_search_text", normalizeAuditTarget(relDir), map[string]string{
		"query": query,
		"hits":  fmt.Sprintf("%d", len(out)),
	})
	return out, nil
}

func (h *Harness) VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error) {
	hits, err := vault.FindBacklinks(h.cfg.Paths.VaultRoot, "", relPath, limit)
	if err != nil {
		return nil, err
	}

	out := make([]model.SearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, model.SearchHit{
			Path:     hit.Path,
			Line:     hit.Line,
			Preview:  hit.Preview,
			DocClass: h.classifier.Classify(hit.Path).Class,
		})
	}

	h.recordReadAudit("vault_backlinks", relPath, map[string]string{
		"hits": fmt.Sprintf("%d", len(out)),
	})
	return out, nil
}

func (h *Harness) DocClassify(relPath string) model.DocClassificationView {
	classification := h.classifier.Classify(relPath)
	matches := make([]string, 0, len(classification.Matches))
	for _, match := range classification.Matches {
		matches = append(matches, fmt.Sprintf("%s:%s", match.Field, match.Pattern))
	}
	view := model.DocClassificationView{
		Path:     cleanRelPath(relPath),
		DocClass: classification.Class,
		Matches:  matches,
	}

	h.recordReadAudit("doc_classify", view.Path, map[string]string{
		"doc_class": string(view.DocClass),
	})
	return view
}

func (h *Harness) ContextPack(targetPath string, task string, limit int) (model.ContextPack, error) {
	if limit <= 0 {
		limit = 6
	}

	managed, err := h.ManagedStatus()
	if err != nil {
		return model.ContextPack{}, err
	}

	systemDoc, err := h.SystemDocGet("system")
	if err != nil {
		return model.ContextPack{}, err
	}
	progressDoc, err := h.SystemDocGet("progress")
	if err != nil {
		return model.ContextPack{}, err
	}
	currentWeek, currentWeekErr := h.currentWeekDocument()

	pack := model.ContextPack{
		Task:        strings.TrimSpace(task),
		TargetPath:  cleanRelPath(targetPath),
		Managed:     managed,
		SystemDoc:   cloneExcerpt(systemDoc, 800),
		ProgressDoc: cloneExcerpt(progressDoc, 800),
		CurrentWeek: cloneExcerptPtr(currentWeek, 800),
	}

	if targetPath != "" {
		targetDoc, err := h.VaultRead(targetPath)
		if err == nil {
			pack.TargetDoc = cloneExcerpt(targetDoc, 800)
			backlinks, err := h.VaultBacklinks(targetPath, limit)
			if err == nil {
				pack.Backlinks = backlinks
			}
		}
	}

	searchQuery := strings.TrimSpace(task)
	if searchQuery == "" && targetPath != "" {
		searchQuery = strings.TrimSuffix(filepath.Base(targetPath), filepath.Ext(targetPath))
	}
	if searchQuery != "" {
		hits, err := h.VaultSearchText(searchQuery, "", limit)
		if err == nil {
			pack.RelatedHits = dedupeHits(hits, limit)
		}
	}

	if pack.CurrentWeek == nil {
		pack.Notes = append(pack.Notes, "no current week execution note was found")
	}
	if currentWeekErr != nil {
		pack.Notes = append(pack.Notes, "current week execution note could not be loaded: "+currentWeekErr.Error())
	}
	if pack.TargetDoc == nil && targetPath != "" {
		pack.Notes = append(pack.Notes, "target document could not be loaded")
	}
	pack.Attachments = mergeAttachments(
		pack.SystemDoc,
		pack.ProgressDoc,
		pack.CurrentWeek,
		pack.TargetDoc,
	)

	h.recordReadAudit("context_pack", normalizeAuditTarget(targetPath), map[string]string{
		"task":  strings.TrimSpace(task),
		"limit": fmt.Sprintf("%d", limit),
	})
	return pack, nil
}

func (h *Harness) currentWeekDocument() (*model.VaultDocument, error) {
	weekDir := filepath.Join("0-\u6392\u671f", "04-\u6267\u884c")
	paths, err := vault.WalkMarkdownPaths(h.cfg.Paths.VaultRoot, weekDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type candidate struct {
		path    string
		modTime time.Time
	}
	candidates := make([]candidate, 0)
	for _, path := range paths {
		classification := h.classifier.Classify(path)
		if classification.Class != model.DocClassPlanWeek {
			continue
		}
		info, err := os.Stat(filepath.Join(h.cfg.Paths.VaultRoot, filepath.FromSlash(path)))
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{path: path, modTime: info.ModTime()})
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})

	doc, err := h.readDocument(candidates[0].path, model.DocClassPlanWeek)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

func (h *Harness) readDocument(relPath string, fallbackClass model.DocClass) (model.VaultDocument, error) {
	data, hash, normalizedPath, err := vault.ReadRelativeWithHash(h.cfg.Paths.VaultRoot, relPath)
	if err != nil {
		return model.VaultDocument{}, err
	}
	docClass := fallbackClass
	if docClass == model.DocClassUnknown {
		docClass = h.classifier.Classify(normalizedPath).Class
	}
	return model.VaultDocument{
		Path:        normalizedPath,
		DocClass:    docClass,
		BaseVersion: hash,
		Content:     string(data),
		Attachments: vault.ExtractAttachmentRefs(string(data)),
	}, nil
}

func (h *Harness) resolveSystemDoc(name string) (string, model.DocClass, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "system", "system_doc":
		return h.cfg.Vault.ManagedCore.SystemDoc, model.DocClassSystemDoc, nil
	case "progress", "progress_index":
		return h.cfg.Vault.ManagedCore.ProgressIndex, model.DocClassProgressIndex, nil
	case "persona":
		return h.cfg.Vault.ManagedCore.Persona, model.DocClassPersona, nil
	default:
		return "", model.DocClassUnknown, fmt.Errorf("unknown system document: %s", name)
	}
}

func (h *Harness) coreDocStatus(name string, relPath string) model.ManagedCoreStatus {
	_, err := os.Stat(filepath.Join(h.cfg.Paths.VaultRoot, filepath.FromSlash(relPath)))
	return model.ManagedCoreStatus{
		Name:   name,
		Path:   relPath,
		Exists: err == nil,
	}
}

func (h *Harness) recordReadAudit(tool string, target string, metadata map[string]string) {
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata["tool"] = tool
	_ = h.auditor.Record(context.Background(), model.AuditRecord{
		ID:         auditID("mcp-read", time.Now()),
		Kind:       model.AuditMCPRead,
		Actor:      currentReadActor(),
		Target:     normalizeAuditTarget(target),
		OccurredAt: time.Now(),
		Metadata:   metadata,
	})
}

func cloneExcerpt(doc model.VaultDocument, limit int) *model.VaultDocument {
	if doc.Path == "" {
		return nil
	}
	cloned := doc
	cloned.Content = excerpt(doc.Content, limit)
	return &cloned
}

func cloneExcerptPtr(doc *model.VaultDocument, limit int) *model.VaultDocument {
	if doc == nil {
		return nil
	}
	return cloneExcerpt(*doc, limit)
}

func excerpt(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func dedupeHits(hits []model.SearchHit, limit int) []model.SearchHit {
	seen := make(map[string]struct{}, len(hits))
	out := make([]model.SearchHit, 0, len(hits))
	for _, hit := range hits {
		key := hit.Path + ":" + fmt.Sprintf("%d", hit.Line)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, hit)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func mergeAttachments(docs ...*model.VaultDocument) []model.AttachmentRef {
	refs := make([]model.AttachmentRef, 0)
	seen := make(map[string]int)
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		for _, ref := range doc.Attachments {
			key := ref.Path + "|" + ref.Kind
			if index, ok := seen[key]; ok {
				if ref.Embed {
					refs[index].Embed = true
				}
				continue
			}
			seen[key] = len(refs)
			refs = append(refs, ref)
		}
	}
	return refs
}

func cleanRelPath(value string) string {
	value = filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
	value = strings.TrimPrefix(value, "./")
	if value == "." {
		return ""
	}
	return value
}

func isMarkdownDocPath(value string) bool {
	value = cleanRelPath(value)
	if value == "" {
		return false
	}
	return strings.EqualFold(filepath.Ext(value), ".md")
}

func normalizeAuditTarget(target string) string {
	target = cleanRelPath(target)
	if target == "" {
		return "vault"
	}
	return target
}

func entryKind(isDir bool) string {
	if isDir {
		return "dir"
	}
	return "file"
}

func currentReadActor() string {
	agentID := strings.TrimSpace(os.Getenv("OBSIDIAN_HARNESS_MCP_AGENT_ID"))
	if agentID == "" {
		return "local"
	}
	return "mcp:" + agentID
}
