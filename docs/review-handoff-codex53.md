# Lore TUI Review 交接文档 — Codex 5.3

> 本文档由 Opus 编写，供 Codex 5.3 接手 TUI 层 code review。
> 截止日期：2026-04-24

---

## 1. 文件地图

| 文件 | 职责 | 行数 |
|------|------|------|
| `internal/tui/interactive_workbench.go` | 顶层 tea.Model：事件循环、鼠标选择、键盘路由、agent 异步调度 | ~438 |
| `internal/tui/interactive_render.go` | 渲染逻辑：对话区、状态面板、审批占位、输入栏 header | ~232 |
| `internal/tui/interactive_theme.go` | 样式常量、wrapText、runeWidth、stripANSI、unescapeLiteralNewlines | ~139 |
| `internal/tui/interactive_render_test.go` | 26 个测试，覆盖渲染、换行、选择、ANSI 剥离 | ~282 |
| `tmp/test_scroll_select.go` | 手动测试用的 viewport 滚动/选择 demo（`//go:build ignore`） | ~60 |

TUI 不碰 `internal/operatoragent/`、`internal/llm/`、`internal/adapter/`、`internal/app/runtime.go`、`internal/harness/`。

## 2. 依赖

| 包 | 版本 | 用途 |
|----|------|------|
| `charmbracelet/bubbletea` | v1.3.5 | TUI 框架 |
| `charmbracelet/bubbles` | v0.21.0 | textarea, viewport, spinner |
| `charmbracelet/lipgloss` | v1.1.0 | 样式/布局 |
| `mattn/go-runewidth` | (transitive) | Unicode 字符宽度（CJK/emoji） |
| `atotto/clipboard` | latest | 跨平台剪贴板写入 |

## 3. 架构决策记录

### 3.1 InteractiveWorkbenchDriver 接口

TUI 通过 `InteractiveWorkbenchDriver` 接口与业务层解耦：

```go
type InteractiveWorkbenchDriver interface {
    Load(lastOutput string) (WorkbenchViewModel, error)
    Execute(line string, lastOutput string) (InteractiveWorkbenchUpdate, error)
}
```

- `Load()` 拉取初始/刷新后的 ViewModel
- `Execute()` 同步阻塞调用 agent，TUI 用 `tea.Cmd` goroutine 包装实现异步
- agent 运行期间 `m.running = true`，禁止二次提交

### 3.2 应用层文本选择（非终端原生）

alt screen + `WithMouseCellMotion()` 捕获鼠标事件后，终端原生选择失效。
解法：程序自己管理选择状态。

- `textSelection` struct 跟踪 active/dragging/startLine/endLine
- 左键按下开始选择，拖动扩展，释放结束
- `Ctrl+C` 有选择时复制（stripANSI → clipboard），无选择时退出
- 高亮用 `lipgloss.NewStyle().Reverse(true)`
- 选择是行级的，不做列级

### 3.3 Edge-scroll

拖选到面板边缘时自动滚动：

```go
const edgeScrollZone = 3
const edgeScrollLines = 3
const paneTopOffset = 2  // border(1) + title(1)
```

- `msg.Y <= paneTopOffset + edgeScrollZone` → `LineUp(3)`
- `msg.Y >= paneBottom - edgeScrollZone` → `LineDown(3)`
- `paneBottom = paneTopOffset + m.chatViewport.Height`

### 3.4 CJK 文本换行

`wrapText()` 在 `interactive_theme.go`，按 rune 遍历，用 `go-runewidth` 计算字符宽度。
中文字符占 2 列，到达 width 时插入 `\n`。

渲染管线：`unescapeLiteralNewlines` → `wrapText(content, contentWidth)` → `indentBlock`

`contentWidth = wrapWidth - 6`（viewport 宽度减去 indent(2) + padding(2) + border(2)）

### 3.5 换行输入

bubbletea v1.x 无法区分 Enter 和 Shift+Enter（终端发送相同字节）。
当前方案：`Ctrl+J` 插入换行。升级 bubbletea v2 后可支持 Shift+Enter（Kitty keyboard protocol）。

### 3.6 智能方向键路由

- `input.LineCount() > 1`（多行输入）→ up/down 发给 textarea
- 单行输入 → up/down 发给 chatViewport 滚动

### 3.7 角色标签去重

连续相同角色的对话不重复显示 "You" / "Lore" 标签，用 `prevRole` 跟踪。

## 4. 已知问题 & 技术债

| 问题 | 严重度 | 说明 |
|------|--------|------|
| `paneTopOffset = 2` 硬编码 | 低 | 假设 border(1) + title(1)。如果布局改动（加 subtitle 等），选择坐标会偏移。DeepSeek 已标记。 |
| ANSI 样式嵌套 | 低 | `applySelectionHighlight` 用 Reverse 包裹已有 lipgloss 样式的行，理论上可能产生嵌套 escape sequence。实测未出问题，但边界情况未验证。 |
| bubbletea v2 升级 | 中 | 升级后可用 Shift+Enter、更好的鼠标事件。但 v2 API 有 breaking changes，需要全面测试。用户已同意延后。 |
| `WithMouseCellMotion` 仅在终端输出时启用 | — | `isTerminalWriter()` 检测 `os.ModeCharDevice`，非终端（pipe/test）不启用鼠标和 alt screen。这是正确行为。 |
| 审批面板是 placeholder | — | `renderApprovalPlaceholder()` 返回固定文本。等 Runtime pending queue 接口落地后接入。 |

## 5. Review 重点

未来改动时重点关注：

1. **坐标系一致性** — 鼠标 `msg.Y` 是终端绝对坐标，viewport 内容行号需要 `YOffset + Y - paneTopOffset` 转换。任何布局改动都要验证这个映射。

2. **contentWidth 计算** — 当前 `wrapWidth - 6`。如果改 indent 深度、padding、border 样式，这个数字要同步更新。

3. **渲染管线顺序** — `unescapeLiteralNewlines` → `TrimSpace` → `wrapText` → `indentBlock`。顺序不能乱，否则缩进会被 wrap 计算吃掉。

4. **选择状态生命周期** — `refreshContent()` 每次都重建 `contentLines` 并重新应用高亮。如果引入增量渲染，选择逻辑需要重新设计。

5. **测试覆盖** — 26 个测试在 `interactive_render_test.go`。新增渲染逻辑必须加测试。`testVisibleWidth()` 可复用于验证 CJK 宽度。

6. **`View()` 里调 `resize()` 和 `refreshContent()`** — 这是因为 Bubble Tea 的 `View()` 是 immutable receiver，不能修改 model。当前实现在 `View()` 里调这两个方法是为了确保布局正确，但它们实际上不会持久化修改（因为 receiver 是值类型）。真正的状态更新在 `Update()` 里。

## 6. 测试命令

```bash
cd C:\Users\15892\Desktop\obsidian-harness
go test ./internal/tui/... -v
go build ./...
```

手动测试 TUI：启动后验证鼠标滚轮、Ctrl+J 换行、Ctrl+C 复制、Tab 切换面板、中文换行不溢出。

## 7. 协作模式

- Opus：TUI 层开发（本文件覆盖的所有代码）
- Codex 5.3：Review TUI + 开发 TUI 外的后端逻辑
- 用户：产品决策、体验验证、最终审批

Review 时如果发现问题，直接标注文件:行号 + 问题描述 + 建议修复方向即可。
