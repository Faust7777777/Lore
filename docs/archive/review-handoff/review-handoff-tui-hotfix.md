# TUI Hotfix 交接文档 — Opus → DeepSeek Review

日期: 2026-04-24

## 改动概述

修复用户实际体验 TUI 后反馈的三个问题：鼠标滚动、复制粘贴提示、`\n` 字面显示。

## 改动文件

### 1. `internal/tui/interactive_render.go`

**新增 `unescapeLiteralNewlines()` (L191-193)**
- 模型在 JSON `message` 字段里输出 `\\n`，`json.Unmarshal` 解码后变成 Go 字符串里的字面 `\n`（backslash + n），不是真正换行符
- 渲染前用 `strings.ReplaceAll(s, backtick\n, "\n")` 清理
- 注意：Go raw string `` `\n` `` 是 backslash + n 两个字符，不是双反斜杠，这是正确的

**应用位置**
- L67: `turn.Content` — 对话历史中的 assistant 回复
- L120: `lastOutput` — 最新输出区域。顺序是先 unescape 再 excerpt，确保长度截断基于渲染后文本

**Input header 更新 (L219)**
- 快捷键提示增加 `Shift+click copy`，告知用户在鼠标捕获模式下如何复制文本

### 2. `internal/tui/interactive_workbench.go`

**鼠标支持 (L63)**
- `tea.NewProgram()` options 增加 `tea.WithMouseCellMotion()`

**MouseMsg 处理 (L155-158)**
- `Update()` 新增 `tea.MouseMsg` case
- 鼠标事件默认转发给 `chatViewport`，不依赖焦点状态
- 设计决策：用户最常滚动的是对话区，不需要先 Tab 切焦点

### 3. `internal/tui/interactive_render_test.go` (新文件)

- `TestUnescapeLiteralNewlines`: 5 个 case 覆盖基本、多次、无转义、空字符串、纯转义
- `TestRenderInteractiveConversationUnescapesNewlines`: 集成测试，验证渲染后不含字面 `\n`
- `TestRenderInteractiveConversationEmptyState`: 空对话显示 ready 提示和 no active output
- `TestRenderInteractiveConversationTruncatesLongHistory`: 超过 8 轮对话只渲染最近 8 轮
- `TestRenderInteractiveConversationShowsToolTrace`: tool trace 区域渲染工具名和错误信息
- `TestRenderInteractiveConversationRunningState`: agent 运行时显示 thinking 和 pending input
- `TestRenderInteractiveConversationLastOutputUnescape`: lastOutput 中的字面 `\n` 被正确转换
- `TestRenderInteractiveStatusBasic`: 状态面板渲染 System 区域、ready 状态、agent ID
- `TestRenderInputHeaderRunning`: 运行时 input header 显示 "agent running"
- `TestRenderInputHeaderIdle`: 空闲时 input header 显示快捷键提示含 Shift+click copy
- `TestRenderApprovalPlaceholder`: 审批面板显示 empty state

## Review 重点

1. `unescapeLiteralNewlines()` 是否会误伤合法内容 — 如果用户或模型真的想显示字面 `\n` 文本（比如讲解转义字符），会被替换掉。当前判断：这种场景极少，且 Lore 是知识管理工具不是编程教学工具，可以接受
2. MouseMsg 只转给 chatViewport — 如果未来 statusViewport 也需要鼠标滚动，需要加焦点判断
3. `Shift+click copy` 提示依赖终端支持（Windows Terminal、iTerm2 支持，老版 cmd.exe 不支持）

## 编译和测试

```
go build ./...                    # 通过
go test ./internal/tui/... -count=1  # 通过
```

## 不碰的文件

- `interactive_theme.go` — 本轮无改动
- `internal/console/*` — runtime 层
- `internal/operatoragent/*` — agent 层
- `internal/llm/*` — provider 层
