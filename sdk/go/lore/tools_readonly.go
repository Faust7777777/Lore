package lore

import "context"

func (c *Client) ManagedStatus(ctx context.Context) (*ManagedStatus, error) {
	var out ManagedStatus
	if err := c.CallTool(ctx, "managed_status", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SystemDocGet(ctx context.Context, req SystemDocGetRequest) (*VaultDocument, error) {
	var out VaultDocument
	if err := c.CallTool(ctx, "system_doc_get", map[string]any{"name": req.Name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) VaultRead(ctx context.Context, req VaultReadRequest) (*VaultDocument, error) {
	var out VaultDocument
	if err := c.CallTool(ctx, "vault_read", map[string]any{"path": req.Path}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) VaultList(ctx context.Context, req VaultListRequest) ([]VaultEntry, error) {
	var out []VaultEntry
	if err := c.CallTool(ctx, "vault_list", map[string]any{"dir": req.Dir}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) VaultSearchText(ctx context.Context, req VaultSearchTextRequest) ([]SearchHit, error) {
	var out []SearchHit
	if err := c.CallTool(ctx, "vault_search_text", map[string]any{"query": req.Query, "dir": req.Dir, "limit": req.Limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) VaultResolve(ctx context.Context, req VaultResolveRequest) (*VaultResolveResult, error) {
	var out VaultResolveResult
	if err := c.CallTool(ctx, "vault_resolve", map[string]any{"query": req.Query, "dir": req.Dir, "limit": req.Limit}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) VaultBacklinks(ctx context.Context, req VaultBacklinksRequest) ([]SearchHit, error) {
	var out []SearchHit
	if err := c.CallTool(ctx, "vault_backlinks", map[string]any{"path": req.Path, "limit": req.Limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ContextPack(ctx context.Context, req ContextPackRequest) (*ContextPack, error) {
	var out ContextPack
	if err := c.CallTool(ctx, "context_pack", map[string]any{"target_path": req.TargetPath, "task": req.Task, "limit": req.Limit}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DocClassify(ctx context.Context, req DocClassifyRequest) (*DocClassification, error) {
	var out DocClassification
	if err := c.CallTool(ctx, "doc_classify", map[string]any{"path": req.Path}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
