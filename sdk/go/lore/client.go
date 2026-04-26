package lore

import "context"

type Options struct {
	Command   string
	WorkDir   string
	ClientKey string
}

type Client struct {
	transport *stdioTransport
}

func Start(ctx context.Context, opts Options) (*Client, error) {
	transport, err := startStdioTransport(ctx, opts)
	if err != nil {
		return nil, err
	}
	client := &Client{transport: transport}
	if err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) Close() error {
	if c == nil || c.transport == nil {
		return nil
	}
	return c.transport.Close()
}

func (c *Client) Initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]any{})
	return err
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.call(ctx, "ping", map[string]any{})
	if err == nil {
		return nil
	}
	var rpcErr *JSONRPCError
	if As(err, &rpcErr) && rpcErr.Code == -32601 {
		return c.Initialize(ctx)
	}
	return err
}

func (c *Client) Tools(ctx context.Context) ([]ToolDefinition, error) {
	var response struct {
		Tools []ToolDefinition `json:"tools"`
	}
	if err := c.callInto(ctx, "tools/list", map[string]any{}, &response); err != nil {
		return nil, err
	}
	return response.Tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any, out any) error {
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	var response toolCallResponse
	if err := c.callInto(ctx, "tools/call", params, &response); err != nil {
		return err
	}
	if response.IsError {
		return &ToolError{Tool: name, Message: response.Text()}
	}
	if out == nil {
		return nil
	}
	return decodeStructuredContent(response.StructuredContent, out)
}

func (c *Client) call(ctx context.Context, method string, params any) (rawMessage, error) {
	if c == nil || c.transport == nil {
		return nil, &TransportError{Op: "call", Err: ErrClosed}
	}
	return c.transport.Call(ctx, method, params)
}

func (c *Client) callInto(ctx context.Context, method string, params any, out any) error {
	result, err := c.call(ctx, method, params)
	if err != nil {
		return err
	}
	return decodeRaw(result, out)
}
