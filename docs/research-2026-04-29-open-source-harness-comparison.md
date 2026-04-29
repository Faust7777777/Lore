# Lore vs. Open-Source Coding Harness: Comparative Research

> 日期：2026-04-29
> 范围：Codex CLI / OpenCode / Gemini CLI
> 目的：明确 Lore 的产品差异，识别可借鉴的工程模式与必须坚守的红线
> 状态：调研报告，不是 ADR；任何代码方向变动前需要先和产品 / Opus 对齐

## 一、根本差异：不是同一类产品

那三家都是 **coding harness**：单元是 *coding session*，安全网是 *git*，治理是 *approval mode*。
Lore 的单元是 *vault under governance*，安全网不是 git 而是 *base_version + 草案状态机*，治理不是审批模式而是 *draft → review → supersede → apply* 多阶段流水。

| 维度 | Codex / OpenCode / Gemini | Lore |
|---|---|---|
| 中心实体 | session / message / file | managed vault + 三件套 |
| MCP 角色 | agent 调外部工具的入口 | **外部 agent 调 Lore** 的入口（read + proposal） |
| 写入治理 | approval mode（一次性） | draft → review → supersede → apply（持久化状态机） |
| 安全网 | git diff + 沙箱 shell | base_version 乐观锁 + post-scan findings |
| 配置中心 | per-project + per-user 多层 | 单层 `config.Default(workDir)`（**这块我们落后**） |
| 工具注册 | `ToolHandler` / `BaseTool` / `ToolInvocation` 接口 | 三处 switch 字符串（schema/dispatch/MCP 各一份，**这块也落后**） |
| 多 provider | OpenCode 12 后端、泛型 + 函数选项 | 只有 `internal/llm/openai/`（Anthropic-native 还是 spec 承诺，未实现） |

`internal/console/tool_runtime.go:75-300` 是 200+ 行的 switch；同一组 case 在 `internal/operatoragent/tool_schema.go` 和 `internal/mcp/tool_contract.go:17-127` 还各写一遍。这是 vault_resolve schema drift（`docs/review-handoff-codex53-vault-resolve.md`）的结构性根因。

## 二、值得借鉴的（按 ROI 排，全都能放进 AGENTS.md 的红线内）

### 高 ROI

**1. 统一 ToolRegistry**（仿 OpenCode 的 `BaseTool` + Gemini 的 `ToolRegistry`）

- 把三处 switch 收成一个 `Tool` 接口：`Name() / Schema() / Run() / RequiresModel() / RequiresAdapter() / AllowedProfiles()`
- 注册表分为 `internalTools`（local console 完整集合）和 `mcpReadOnlyTools`（MCP 子集）
- 这正好把决策 #13 §运行时属性声明（`requires_model` / `requires_adapter` / `allowed_profiles`）落到接口层
- 影响面：`internal/console/tool_runtime.go`、`internal/operatoragent/tool_schema.go`、`internal/mcp/server.go`、`internal/mcp/tool_contract.go`
- 不破冻结契约（`Response` / `ToolCallTrace` 不动），是纯重构

**2. 配置分层**（仿 Codex 的 `ConfigLayerStack`，**不要**仿到 cloud 层）

- 现在 `internal/config/config.go:65-112` 只有 `Default(workDir)`
- 项目说明 §10 明文要求"发行版骨架 + 用户覆盖"，但代码里没 layering
- 加 `defaults → ~/.lore/config.json → <workdir>/.lore/config.json → env`，按字段合并而不是整层替换
- 顺便解决 `LORE_LLM_BASE_URL` 这类环境变量目前没有正式合并路径的问题

**3. PendingActionQueue**（仿 Gemini 的 `PolicyEngine`，**不要**仿 `auto_edit/yolo` mode）

- DEVELOPMENT_STATUS 已写明 TUI pending approval 是占位
- 设一个 runtime 级队列：`shell_exec confirm` / `draft approve prompt` / 未来的 `low_risk_write confirm` 都进同一个队列
- 队列项就是 view model，TUI（Opus 域）只渲染、不发明语义，正合 AGENTS.md "Approval and confirmation must be runtime-backed"
- **红线**：不能学 Gemini 的 `yolo`、`auto_edit`——治理文档的 draft→review→apply 是产品硬约束

### 中 ROI

**4. Provider 接口 + 函数选项**（仿 OpenCode 的 `baseProvider[C]`）

- 单独建 `internal/llm/provider.go`：`type Provider interface { Chat / ChatStream / ListModels }`
- 现在的 OpenAI 客户端实现这个接口；加 Anthropic-native 时不动 operatoragent
- 项目说明 §10 承诺 `Responses-compatible` 和 `Anthropic-native` 双协议，目前只有前者

**5. Turn/Session 状态分离**（仿 Codex 的 `SessionState` + `ActiveTurn`）

- `internal/operatoragent/model.go` 31KB 单文件
- 拆成 `Session`（多 turn）和 `Turn`（单次 input→tools→final）能让事件粒度细化、TUI 异步桥更干净
- **风险**：可能影响冻结契约 `operatoragent.Response`——动之前要先和 Opus 对齐，按 AGENTS.md "If [a frozen contract] must change: state why / identify consuming layers / update tests in the same change"

**6. 细粒度事件流**（仿 Gemini 的 17-event AsyncGenerator，**不要**真做到 17 个）

- 现有 `internal/runtime/events.go` 只有粗粒度（FileChange、DraftStateChange、DailyRollupWritten）
- 加 5–6 个 turn 内事件：`TurnStarted` / `ToolCallRequested` / `ToolCallCompleted` / `AssistantDelta` / `TurnFinished`
- TUI workbench 可以渲染逐步进度，不必等整个 turn 结束

### 不建议借鉴

- **Codex 的 OS 沙箱**（Seatbelt / Landlock / bwrap）：Lore 在 MCP 不暴露 shell，本地 shell_exec 已 confirm-gated，加 OS 沙箱是过度工程
- **Gemini 的 HierarchicalMemory + 自动记忆改写**：直接违反"画像/进度更新必须经草案"
- **OpenCode 的 LSP**：Lore 的客户端是 Obsidian 不是 IDE
- **Gemini 的 extension 市场 / 多 subagent isolation**：会把项目推向"通用 agent 平台"，AGENTS.md §Scope 明确禁止
- **OpenCode 的 12 provider**：超出 spec 承诺范围，不是产品价值点
- **Codex 的 cloud config layer**：Lore 是 local-first single-user 产品

## 三、Lore 反过来领先的地方（不要被开源项目牵着改）

- **草案状态机 + supersede**：那三家都没有，approval mode 是一次性的；我们的 `created → pending_review → approved → applied`，加 `superseded` / `conflicted` 是正经的过程治理
- **AuditRecord 结构化 + reason_codes**：他们都有日志，但没把审计当一等公民
- **post-scan + findings**：他们假设 git 是安全网，不治理越带写入；我们做了
- **三件套 + agent.md/identity.md 双文档分层**：核心治理层独有
- **base_version 乐观锁 + 原子写**：他们直接信任 fs，不处理 Obsidian Sync 共存

这些不要在重构中被稀释。

## 四、推荐改造次序

1. ✅ 先 commit 现有 worktree 的 v1 governance feature line（已完成，4 个 commit）
2. **统一 ToolRegistry**——独立切片，纯重构 + 测试，消灭三处 switch drift
3. **配置分层**——独立切片，新加文件多、改动小，对应 spec §发行版+覆盖
4. **PendingActionQueue**——和 Opus 对齐 view model 后做
5. Provider 抽象 / Turn 分离 / 事件细粒度——评估后再决定哪个先做

## 五、参考索引

- 评审对齐文档 §六（开源 harness 参考）：[`Obsidian Vault/05-项目文档/Obsidian Harness 评审对齐文档.md`](../../Obsidian%20Vault/05-%E9%A1%B9%E7%9B%AE%E6%96%87%E6%A1%A3/Obsidian%20Harness%20%E8%AF%84%E5%AE%A1%E5%AF%B9%E9%BD%90%E6%96%87%E6%A1%A3.md)（1004-1029 行）
- AGENTS.md §Architecture Rules / §Frozen Contracts
- ADR：`docs/adr-lore-v1-architecture.md`
- 当前规划：`docs/plans/2026-04-28-external-agent-governed-intake.md`
- 上一任开发者交接：`docs/handoff-next-developer-2026-04-29.md`
