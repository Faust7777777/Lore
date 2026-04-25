# 主 Agent 交互模型对齐稿（三方评审版）

> 背景：评审对齐文档 #13 定义了工具注册和 MCP 暴露边界，但没有定义主 Agent 自身的交互模型。当前 console 实现是单动作 operator（用户说一句 → LLM 翻译成一个 action → 执行），这不是产品目标。本文档补齐这个空白，经三方（用户、Opus、Codex）评审后冻结，写回评审对齐文档作为 #13 补充条款。
>
> 评审记录：
> - Opus 初稿 → Codex 提出 6 条修正 → 用户确认 Codex 修正方向 → Codex 补充低治理写入口分析 → 三方合并本版

---

## 1. 主 Agent 定位

主 Agent 是**知识运营主 Agent**，交互形态采用**对话式 agent loop**，而不是单动作 operator。

它不是通用 agent 平台（对齐文档 1.3 明确排除）。"像 Codex/OpenCode"指的是对话 + 多步工具循环的交互模型，不是默认开放任意通用工具的能力范围。

核心特征：
- 能理解自然语言对话，支持多轮交互
- 默认优先使用 Lore 预设工具（vault 读写、草案、process-sink、分类、搜索等）
- 倾向于分析、建议、生成草案，而不是直接执行
- 知识运营是主业，不是附带功能

---

## 2. 能力注册 vs 行为策略 vs 硬边界（三层控制）

当前实现的核心误解：把"MCP 不暴露 shell"等同于"主 Agent 也不能用 shell"，把工具注册当成行为约束。

正确的三层控制模型（对应 #13 的三层分离）：

| 控制层 | 职责 | 举例 |
|---|---|---|
| 能力注册（Agent 工具层） | 声明 agent 可以调用哪些工具。按 phase/profile 注册，不是全量注册 | P0 只注册 Lore 预设工具 |
| 行为策略（System Prompt） | 控制 agent 的默认行为偏好 | "默认优先使用 Lore 工具" |
| 硬边界（Runtime-Policy-Operator 层） | 强制执行不可绕过的治理规则 | vault 治理文档写入必须走 draft/approval，无论 agent 或用户怎么说 |

**关键原则：**
- 工具注册是能力声明
- 默认行为由 system prompt 控制
- 硬边界由 runtime/policy/approval 强制执行
- 三层各司其职，不能只靠 prompt 一层

---

## 3. 工具调用策略（两层 + 未来扩展）

### 第一层：默认主动（无需用户确认）

这些工具在对话中可以自由调用：

- `vault_read` / `vault_list` / `vault_search_text` / `vault_backlinks`
- `system_doc_get` / `managed_status` / `doc_classify`
- `context_pack`
- `checkpoint_status` / `daily_report_status`
- `draft_list` / `draft_get`

特征：全部是只读操作，不改变任何状态。

### 第二层：建议后执行（agent 提出，用户确认后执行）

这些工具 agent 可以主动提议使用，但必须停在确认/审批点：

- `draft_create` — 生成草案
- `draft_approve` / `draft_reject` — 草案状态变更
- `controlled_apply` — 草案写回 vault
- `vault_write_low` — 低治理文档直写（见第 8 节）
- `checkpoint_regenerate` / `daily_rollup_trigger` — 手动触发 process-sink 操作

特征：会改变系统状态，但都在 Lore 治理模型内，受 runtime 硬边界保护。

### 第三层：明确请求时激活（P1 注册，上线必须包含）

以下能力是**上线范围内的正式能力**，P1 注册，不是永久排除也不是"以后再说"：

- shell 命令执行
- 直接编写/修改代码文件
- git 操作

P0 不注册这些工具（P0 聚焦两条主链路闭环）。P1 注册到主 Agent 聊天工具面（内部 tool dispatch，不经过 MCP），受行为策略约束：默认不主动使用，用户给出明确执行意图时才激活。

**触发条件**：用户消息中包含明确的执行意图，例如：
- "帮我改一下这个函数"
- "跑一下这个命令"
- "帮我实现这个功能"

**不触发的情况**：
- "这个函数有什么问题" → 分析，不改代码
- "这个怎么实现比较好" → 建议，不写代码

**与 #13 的关系：**
- #13 写的"research-exec v1 不进入主工具面"指的是 MCP 工具面，不是主 Agent 内部工具面
- MCP 始终不暴露 shell（不变）
- 主 Agent 内部 shell/coding 能力 P1 注册，上线时可用
- runtime 层仍然强制 vault 治理文档走 draft/approval，shell/coding 不能绕过

---

## 4. vault 写入规则（硬边界）

这是 runtime 层强制执行的规则，不受 system prompt 或用户自然语言请求影响：

| 文档治理等级 | 写入方式 | 能否绕过 |
|---|---|---|
| 最高（三件套） | 必须 draft → approval → apply | 不能。任何情况下都不能直写 |
| 高（计划/执行） | 默认 draft → approval → apply | 不能。即使用户说"直接改" |
| 中（模板） | 可配置 | 按配置 |
| 过程沉淀（时报/日报） | 受控自动写入 + 审计 | 不走 draft，但只有 runtime 内部 process-sink 可写 |
| 低（知识笔记） | 可按规则直写 | 走 `vault_write_low`，runtime 校验后允许 |

**关键约束：vault 治理文档的 draft/approval 是硬约束，不因自然语言请求豁免。**

---

## 5. 对话模型

### 5.1 多轮对话，不是单动作

当前 console 实现：用户说一句 → LLM 输出一个 JSON action → 执行 → 打印结果 → 等下一句。

目标模型：用户说一句 → agent 可能调用多个工具、做多步分析、生成结构化回复 → 用户可以追问、修正、确认 → agent 继续。

### 5.2 一轮内的自主步数

agent 在一轮回复中可以：
- 调用多个只读工具收集信息（第一层，无限制）
- 基于收集的信息做分析和建议
- 提出一个或多个操作建议（第二层，需确认）

agent 在一轮回复中不应该：
- 未经确认连续执行多个状态变更操作
- 自主决定进入 coding/shell 模式（P0 不注册；P1 注册后仍需用户明确请求）
- 在用户没有要求的情况下主动修改文件

### 5.3 上下文保持

agent 在一个 session 内保持对话上下文。不是每句话都从零开始理解。

---

## 6. 与已有决策的关系

| 已有决策 | 本文档的补充 |
|---|---|
| #13 工具表 12 组 | 12 组是能力注册。本文档定义调用策略（两层 + P1 第三层） |
| #13 shell_command 结论 | "MCP 不暴露 shell"不变。主 Agent 内部 shell/coding 能力 P1 注册，上线时可用，受第三层策略约束 |
| #13 5 Profile | Profile 控制"谁能调用什么"。本文档补充"什么时候主动调用" |
| #13 三层分离 | 本文档的三层控制（能力注册 / 行为策略 / 硬边界）对应 #13 的三层分离（Agent 工具层 / Runtime-Policy-Operator 层 / TUI-Human-Review 层） |
| 2.2 治理等级表 | 本文档补齐低治理文档的直写工具入口（`vault_write_low`），填补治理模型允许但工具层缺失的缺口 |
| 2.5 主动系统边界 | "默认产出是分析和草案"不变。本文档把这个原则从写入策略扩展到对话策略 |
| #4 daemon 进程管理 | daemon 的后台行为（定时、attach、ingest、巡检）不受本文档影响。本文档只定义用户主动对话时的交互模型 |

---

## 7. 对当前实现的具体修改方向

### 7.1 console 命令重构

当前：
```
用户输入 → LLM 翻译成 {"action": "xxx"} → switch/case 执行 → 打印结果
```

目标：
```
用户输入 → LLM 收到 system prompt（含策略规则）+ 工具定义（按 phase/profile 注册）+ 对话历史
→ LLM 自主决定调用哪些工具（受策略 + runtime 硬边界双重约束）
→ 工具结果回注 LLM
→ LLM 生成自然语言回复（可能包含建议、分析、操作结果）
→ 用户继续对话
```

这是标准的 agent loop（参考 OpenCode 的 `processGeneration` 循环、Codex 的 `ActiveTurn` 循环）。像的是交互模型，不是能力范围。

### 7.2 system prompt 需要包含

1. 角色定义：你是 Lore 的知识运营主 Agent
2. 两层策略规则（第 3 节的内容）
3. vault 治理规则摘要（从系统说明中提取）
4. 当前系统状态（managed mode / blocked / degraded）
5. 硬边界声明（第 4 节的写入规则，提醒 LLM 这些规则由 runtime 强制执行）

### 7.3 P0 工具注册

P0 只注册 Lore 预设工具（已实现的 #13 工具组子集）：
- vault-access 组：`vault_read`、`vault_list`
- vault-search 组：`vault_search_text`、`vault_backlinks`
- system-core 组：`system_doc_get`、`managed_status`
- document-classify 组：`doc_classify`
- context_pack
- drafts 组：`draft_list`、`draft_get`、`draft_create`
- controlled-apply 组：`draft_approve`、`draft_reject`、`controlled_apply`
- process-sink 组：`checkpoint_status`、`daily_report_status`、`checkpoint_regenerate`、`daily_rollup_trigger`
- 新增：`vault_write_low`（见第 8 节）

P0 不注册：shell_exec、file_write、file_read（通用）、git 操作。

---

## 8. 低治理写入口：`vault_write_low`

### 问题

对齐文档 2.2 治理等级表定义了"低治理文档可按规则直写"，但 #13 的 12 个工具组里没有对应的写入工具。当前代码中唯一的 vault 写入路径是 draft → apply 和 process-sink 内部写入。

这导致一个真实场景无法闭环：用户说"根据我的画像和今天日报写一篇日记"，agent 能读、能生成，但没有工具把日记写进 vault。

### 解法

新增 `vault_write_low` 工具，属于 vault-access 工具组的写入扩展：

- 只允许写入低治理等级的文档类（知识笔记等）
- runtime 层校验：`doc_classify(target_path)` → 治理等级 → 低等级才放行
- 高治理等级（三件套、计划、执行）一律拒绝，返回 `status: unauthorized`，提示走 draft/approval
- 中等级（模板）按配置决定
- 写入方式：原子写（与所有 vault 写入一致）
- 写入后记审计日志
- 不走 draft/approval 流程

### 为什么不用 draft 自动审批

让低治理文档也走 draft → auto-approve → apply 在形式上更保守，但：
- 增加了不必要的状态机复杂度
- 用户体验差（写一篇日记要等 draft 流转）
- 对齐文档已经明确说了"可按规则直写"，不是"可按规则走简化审批"

### 完整链路示例

```
用户："根据我的画像和今天日报写一篇日记"
→ agent 调用 vault_read（画像）— 第一层，自由调用
→ agent 调用 vault_read（日报）— 第一层，自由调用
→ agent 生成日记内容（LLM）
→ agent 提议调用 vault_write_low — 第二层，需用户确认
→ 用户确认
→ runtime 校验：doc_classify(目标路径) → 知识笔记 → 低治理 → 放行
→ 原子写入 + 审计日志
→ 完成
```

### Phase 归属

`vault_write_low` 应进入 P0。理由：
- 它补齐的是治理模型已经允许但工具层缺失的能力
- 没有它，主 Agent 在对话中只能读不能写，"知识运营"退化为"知识查询"
- 实现量很小：runtime 层加一个 doc_classify 校验 + 复用已有的原子写入

---

## 9. 需要冻结的 6 个点

经三方评审，以下 6 点冻结：

1. **主入口定位**：知识运营主 Agent，采用对话式 agent loop，不是单动作 operator
2. **工具边界**：按 phase/profile 注册；默认优先 Lore 工具；行为策略 + runtime/policy 双控
3. **coding 触发**：P0 不注册；P1 注册到主 Agent 内部工具面，上线必须包含。默认不主动使用，用户明确请求时激活。MCP 始终不暴露 shell
4. **vault 写入**：治理文档严格走 draft/approval，不因自然语言请求豁免；低治理文档走 `vault_write_low`（runtime 校验 + 审计）
5. **后台行为边界**：定时/attach/ingest/巡检归 daemon，不进入聊天主入口
6. **一轮自主步数**：允许多步只读与分析；状态变更必须停在确认/审批点

---

## 10. 实现优先级

1. **P0**：把 console 从单动作 operator 改成标准 agent loop + system prompt 策略。注册 Lore 预设工具（第 7.3 节列表）。实现 `vault_write_low`。
2. **P1（上线前置条件）**：注册 shell/coding/git 到主 Agent 内部工具面（不经过 MCP），加入第三层策略约束（默认不主动用，明确请求才激活）。补齐多轮上下文管理。**这不是可选增强，是上线必须完成的能力。**
3. **P2**：更精细的策略控制（per-profile 策略、用户自定义策略规则）。

---

## 11. Session Transcript and Resume (Frozen Addendum)

This section freezes the Lore session transcript / resume behavior. It is separate from process-sink, audit logs, and cross-session memory.

### 11.1 Product Semantics

Lore records local session transcripts for resume and debugging.

Transcripts are not memory:

- A new chat MUST NOT load previous transcripts by default.
- Previous transcripts are read only when the user explicitly asks for resume or history search.
- `--resume` continues a specific prior session context; it is not global long-term memory.
- History search is explicit user action, not automatic prompt injection.

### 11.2 Storage Location

P0 storage is workspace-local:

```text
<workdir>/state/sessions/
  index.json
  lore-YYYYMMDD-HHMMSS-<suffix>.jsonl
```

Session history is scoped to the current `--workdir`. Sessions from different workdirs do not automatically share history.

A global session index such as `~/.lore/sessions-index.json` is out of P0 scope.

### 11.3 Format

Session transcripts use append-only JSONL. Each line is one JSON event written with a JSON encoder, never hand-built strings.

P0 event types:

- `session_meta`
- `user_message`
- `assistant_message`
- `tool_call`
- `working_set`
- `local_command`
- `error`
- `session_end`

Example shape:

```json
{"type":"session_meta","version":1,"session_id":"lore-20260424-153012-a8f3","timestamp":"2026-04-24T15:30:12+08:00","agent_id":"lore","model":"gpt-5.4"}
{"type":"user_message","timestamp":"2026-04-24T15:30:18+08:00","text":"show persona"}
{"type":"tool_call","timestamp":"2026-04-24T15:30:19+08:00","name":"system_doc_get","arguments":{"name":"persona"},"status":"ok"}
{"type":"assistant_message","timestamp":"2026-04-24T15:30:20+08:00","text":"# 人物画像\n..."}
```

### 11.4 Index

`index.json` is a workspace-local acceleration index for listing recent sessions.

It stores summaries such as:

- session id
- relative transcript path
- started_at / updated_at
- title
- turn count
- model
- agent id

The transcript JSONL remains the source of truth. The index may be rebuilt from transcripts if needed.

P0 title generation is deterministic and cheap:

- use the first `user_message`
- trim whitespace
- replace newlines with spaces
- limit to 40 runes
- fall back to session id if missing

### 11.5 CLI Behavior

P0 flags:

```text
--resume       list recent 20 sessions and choose interactively
--resume-id    resume the specified session id directly
```

Avoid `--session-id` in P0 to reduce naming overlap with the future `lore sessions` command.

Interactive selection belongs in the CLI entry layer, not in `console.Session`.

Expected commands:

```powershell
lore tui --workdir <dir>
lore tui --workdir <dir> --resume
lore tui --workdir <dir> --resume-id lore-20260424-153012-a8f3
lore console --workdir <dir> --resume-id lore-20260424-153012-a8f3
```

Future commands may include:

```powershell
lore sessions --workdir <dir> --recent 20
lore sessions --workdir <dir> --search "persona"
```

### 11.6 Resume Semantics

Resume reconstructs session state, but never replays old tool calls.

P0 restore behavior:

- rebuild `console.Session.History` from `user_message` and `assistant_message`
- keep the existing prompt cap behavior, currently recent history only
- restore `WorkingSet` from the latest persisted `working_set` event
- do not inject unrelated old transcripts into new sessions
- continue appending to the same JSONL transcript after resume

WorkingSet recovery is best-effort. It restores the latest persisted working_set snapshot, not necessarily the last in-memory state if the process crashed before the snapshot was written.

### 11.7 Safety and Size Limits

The `sessionlog` layer owns truncation and sanitization for tool events. Callers must not be trusted to trim safely.

P0 limits:

- user and assistant message text: keep complete content
- tool arguments JSON: truncate to a bounded size, default 4096 bytes
- tool error text: truncate to a bounded size, default 2048 bytes
- tool results are not recorded unless explicitly modeled later

This avoids transcript blow-up from large write payloads or accidental oversized arguments.

### 11.8 Package Boundaries

Add a dedicated package:

```text
internal/sessionlog/
  model.go
  writer.go
  reader.go
  index.go
  title.go
```

`console` defines a small recorder interface and does not depend on concrete storage:

```go
type TranscriptRecorder interface {
    RecordUser(text string) error
    RecordAssistant(text string) error
    RecordToolTrace(trace []operatoragent.ToolCallTrace) error
    RecordWorkingSet(items []operatoragent.WorkingSetItem) error
}
```

`sessionlog` may depend on `operatoragent` for frozen data types such as `ConversationTurn` and `WorkingSetItem`.

`operatoragent` must not depend on `sessionlog`.

### 11.9 Corruption Tolerance

The JSONL reader must tolerate partial or corrupted lines:

- empty lines are skipped
- invalid JSON lines are skipped
- a truncated final line must not make resume fail
- tests must cover corrupted-line recovery

### 11.10 Relationship to Other Logs

Do not mix these concerns:

- `sessionlog`: local full conversation transcript for resume/debug/search
- `process-sink`: checkpoint/daily operational knowledge output
- `audit`: governance and safety accountability records

Session transcripts live under state, not in the vault by default.

Writing a session summary into the vault is a future explicit action, not automatic transcript persistence.

### 11.11 P0 Acceptance Criteria

P0 is complete when:

- each new TUI/console session creates a JSONL transcript under `<workdir>/state/sessions/`
- new sessions do not load previous transcripts by default
- `--resume-id` restores a specified session
- `--resume` lists the most recent 20 workspace-local sessions in the CLI entry layer
- resumed sessions append to the same transcript
- WorkingSet is restored from the latest snapshot
- tool trace arguments/errors are truncated in `sessionlog`
- corrupted JSONL lines do not break resume
- TUI shows the current session id or transcript path somewhere discoverable