# Full Project Audit — 2026-07-07

本页是 2026-07-07 对 `obsidian-harness`（代号 Lore）的一次系统性代码审阅结论，落成仓库内文档供后续按优先级修复。审阅以**本地 working tree 为事实来源**，由 B 线主审（Claude Fable 5）主导读码，并调用 WSL 内的 Codex CLI 作只读交叉审。

> 状态：**审阅完成、尚未修复**。本文档只记录发现与修复方向，不含任何代码改动。修复留待后续按第 10 节路线图逐条处理。

## 0. Review Baseline

| Item | State |
| --- | --- |
| Repo | `C:\Users\15892\Desktop\obsidian-harness` |
| Reviewed branch | `b-line/audit-followups-2026-06-04` |
| Reviewed HEAD | `a8ae6ff docs: avoid volatile rc head hash` |
| Merge-base with main | `068b113` |
| Worktree before this doc | clean |
| Tests | `go test ./...` 全绿（29 包，本次会话已跑） |
| Reviewers | Claude Fable 5（主审，本地读码）+ Codex CLI 0.142.5（WSL，只读 `--sandbox read-only --ephemeral` 交叉审） |
| Scope | vault 写入安全、orchestrator 治理、MCP/tools 边界、store/import 链路、密钥处理、编码、测试缺口 |

## 1. 总体结论

**整体质量高，且明显经过多轮安全导向加固。** vault 写入有分层路径/符号链接校验、原子写带 fsync + 跨设备回退、apply 路径互斥串行化并对写后状态失败落 governance Finding、密钥从不落盘、MCP 严格只读+提案。`go test ./...` 全绿。

**架构边界基本清晰**，一处例外见 P1-1。三条操作面（TUI / CLI / MCP）职责分明；`internal/tools` 用 Surface 位掩码把"谁能调什么"做成可验证的单一事实源，是全项目最稳的设计。

**最大的 5 个风险：**

1. **（P1-1）process-sink 写入路径可越界**：`--agent` 的 `agentID` 经 `applyCodexJSONLIdentity` 覆盖后未再消毒，直接进 `filepath.Join(rootDir, agentID, ...)`，`../` 可把 checkpoint/daily 的 .md 写到 sink 目录甚至 workdir 之外。这是**唯一绕过 vault root/符号链接校验的写路径**（裸 `WriteFileAtomic`）。主审与 Codex 独立均定位到此。
2. **（P1-2）AI 治理边界比 README 承诺软**：`vault_write_low` 在默认 console/TUI 画像即对模型开放（不需 `--local-exec`），且 `draft_approve`/`draft_apply` 也是模型可直接调用、**唯一有人类确认的是 `shell_exec`**。本地 agent 可自建草稿→自批准→自 apply，对普通笔记还能 `overwrite:true` 直接覆盖，无 diff/预览/回滚。
3. **（P2-1）vault 搜索/反链在长行文件上整体失败**：`vault/query.go` 的 `bufio.Scanner` 未设大缓冲，单行 >64KB 会让 `vault_search_text`/`vault_backlinks` 报错。`sessionlog`、`externaljsonl` 已修同类问题（见 `docs/archive/review-handoff/review-handoff-codex53-sessionlog-large-lines.md`），vault 层漏了——明确的一致性漂移。
4. **（P2-2）`--resume-id` 路径穿越**：`sessionlog.Load` 用未校验 sessionID 拼路径，`../../x` 可读取并追加 JSONL 到 session 根之外。历史 resume-cli-p0 只覆盖了 TTY/失败语义，未覆盖此向量。
5. **（P2-4）CLI 位置参数不设防**：`resolveWorkDir` 把首个位置参数直接当 workdir，`lore bootstrap --workdir ./real` 会创建名叫 `--workdir` 的目录并静默忽略 `./real`——仓库根现存的 `--help/`、`--workdir/` 垃圾目录即为此产物。

**是否适合继续迭代：适合。** 无一需推倒重来，全部是边界收口，修复面小，且项目内已有可照抄范式（`sanitizeAgentID`、`ResolveWriteUnderRoot`、`scanner.Buffer`）。**优先修** P1-1 与 P1-2（覆盖既有笔记需确认那半件）。

## 2. 项目架构地图

**用途**：local-first、以治理为核心的 Obsidian 知识操作 agent。核心规则：重要 vault 变更先提案、本地审阅、再 apply。主面是 TUI；CLI 作自动化/诊断/模型工具面；MCP 是外部 agent 的只读 + 提案入口（非写代理）。

**主入口**：`cmd/lore`（首选）、`cmd/obsidian-harness`（兼容）。

**数据流：**

```
用户输入 (TUI/console)
  → internal/operatoragent (模型 loop + 工具协议解析)
  → internal/console/tool_runtime.go (读工具 / 草稿动作 / vault_write_low / --local-exec 工作区&shell)
  → internal/orchestrator/harness.go (draft 状态机 / apply / 低治理直写边界 / audit / findings)
  → internal/vault (路径安全 + 原子写 + 搜索/反链/附件)
  → internal/store/sqlitestore (state/store.db)

外部 agent → internal/mcp/server.go (仅 SurfaceMCP：9 读 + 2 提案) → harness.ProposeXxx → 草稿
Codex/外部 transcript → internal/adapter/* → internal/app/import_* → processsink → checkpoint/daily .md
```

**文件写入点（全量 mutation-site 扫描结论）：**

- 走安全边界 `WriteRelativeAtomic → ResolveWriteUnderRoot` 的：bootstrap 模板、ApplyDraft、WriteLowRiskNote、sessionlog 索引 —— ✅ lexical 根校验 + Lstat 拒符号链接 + EvalSymlinks + 逐级祖先符号链接检查。
- 不走该边界、裸 `WriteFileAtomic` 的：process-sink `fileWriter.Write`（`harness.go:969`，路径来自 `checkpointPath`）；`--local-exec` 的 workspace_write/edit（自带 `resolveWorkspacePath` 约束）。**前者是 P1-1 根因**。
- 删除：全项目**无** `os.RemoveAll`；`os.Remove` 仅用于 config temp、原子写 temp、codex sync 锁清理 —— 无用户数据删除路径。

**密钥**：只存环境变量名（`api_key_env`），从不存值；`Diagnostic()` 剥离 `APIKey`，`SanitizeLLMBaseURL` 去除 URL 内凭证。

## 3. 关键风险清单

### P1-1 · process-sink · agentID 路径穿越写越界
- **证据**：`internal/domain/processsink/service.go:118-125`（`checkpointPath`/`dailyReportPath` 把 `agentID` 直接 `filepath.Join(rootDir, agentID, ...)`）；`internal/orchestrator/harness.go:968`（`fileWriter.Write` 裸 `WriteFileAtomic`，不经 `ResolveWriteUnderRoot`）；`internal/app/import_codex.go:60-75`（`applyCodexJSONLIdentity`：`transcript.AgentID = strings.ToLower(agentID)` 覆盖后**未**再 `FinalizeTranscript`/`sanitizeAgentID`）；对照 `internal/app/import_external.go:67-75`（覆盖后**有**调 `FinalizeTranscript` → `sanitizeAgentID`，`parser.go:114`）。
- **问题**：`lore import-codex-jsonl|sync-codex-jsonl|attach-codex-jsonl|import-codex-appserver --agent "../../evil"` 会让 checkpoint/daily .md 写到 `rootDir/../../evil/...`。`filepath.Join` 只 Clean 不约束；`sanitizeAgentID`（只留 `[a-z0-9-_]`，丢 `.`/`/`）本可挡，但该覆盖路径没经过它。process-sink 是唯一无独立 root 约束的写路径，下游无兜底。
- **为什么重要**：直接违反"写入不越界、不污染 vault"核心不变量。触发者虽是本地操作者（自伤性、真实概率低），但同类问题另一条路径已被测试覆盖、这条没有。
- **建议修复**：`applyCodexJSONLIdentity` 覆盖后补 `codexjsonl.FinalizeTranscript(transcript)`（与 external 对齐，一行）；更稳的做法是 process-sink 写入改走 `ResolveWriteUnderRoot`（要求 `process_sink_dir` 在 vault_root 下）。
- **需新增测试**：是。

### P1-2 · console/orchestrator · 本地 AI 写入/批准无强制人类确认
- **证据**：`internal/console/tool_runtime.go:54`（`vault_write_low` 在 `DescribeTools` 基础工具列表，位于 `if r.localWorkToolsEnabled()` 之前）；`tool_runtime.go:283-297`（`CallToolContext` 处理 `vault_write_low` 无 gate）；`harness.go:679-717`（`WriteLowRiskNote`：`overwrite=true` 时无 baseVersion 校验、直接原子覆盖）；`tool_runtime.go:519-541`（**只有** `shell_exec` 设 `PendingShellCommand`；`draft_approve`/`draft_apply`/`draft_supersede`/`vault_write_low` 全 inline 执行）。
- **问题**：默认 `lore tui`/`console`（无 `--local-exec`）下模型即可：(a) 写/覆盖普通 `.md`（治理核心/计划/process-sink/隐藏目录/非 md 被 `validateLowGovernanceMarkdownTarget` 挡，普通笔记不挡，`overwrite:true` 覆盖既有）；(b) 自建草稿→approve→apply 一条龙自我批准落盘。README 头条"proposed first, reviewed locally, and only then applied"对本地 agent 实为靠人盯 TUI，而非强制闸。
- **为什么重要**：本项目"AI 边界"的核心决策点。有全审计、外部 MCP 也进不来（缓解到位），但普通笔记覆盖无 diff/预览/回滚，模型误判即静默丢失用户编辑。
- **建议修复**（择一，按侵入度递增）：① `vault_write_low` 对既有文件的 `overwrite` 走 `shell_exec` 式 pending-confirm；② 覆盖强制走草稿；③ 至少在 audit metadata 记被覆盖内容的 sha256 使覆盖可追溯。**属产品语义决策，建议先拍板方向再动手。**
- **需新增测试**：是。

### P2-1 · vault · 搜索/反链在长行文件上整体失败
- **证据**：`internal/vault/query.go:104`（`SearchText`）与 `:172`（`FindBacklinks`）用 `bufio.NewScanner(file)` 但**无** `.Buffer()`；对照 `internal/sessionlog/writer.go:323,449` 与 `internal/adapter/externaljsonl/parser.go:44` 均调用 `scanner.Buffer(..., MaxJSONLLineBytes/maxExternalJSONLLine)`。历史修复见 `docs/archive/review-handoff/review-handoff-codex53-sessionlog-large-lines.md`（sessionlog 已用 16 MiB 上限替换硬编码 1 MiB，理由与此完全同类）。
- **问题**：任一 vault `.md` 含单行 >64KB（内嵌 base64 data-URI 图、长 HTML 表、粘贴压缩文本）时 `scanner.Scan()` 返 `bufio.ErrTooLong`，回调上抛，`WalkDir` 中断，**整个搜索/反链查询失败并丢弃已命中结果**。MCP 外部 agent 与本地 agent 都依赖这两个读工具。
- **为什么重要**：读路径可用性缺口，且是明确的一致性漂移——同一 bug 类别别处修了、这里漏了（`review-handoff-b-line-vault-search-and-backlinks-2026-05-25.md` 那次覆盖也未含此防护）。
- **建议修复**：两处各加 `scanner.Buffer(make([]byte, 0, 64*1024), maxLine)`（`maxLine` 取宽松上限，如 8–16 MiB），或对超长行降级跳过该文件而非中断整次查询。
- **需新增测试**：是（长行 .md 下搜索仍返回其余命中）。

### P2-2 · sessionlog · `--resume-id` 路径穿越
- **证据**：`internal/sessionlog/writer.go:305-311`（`Load`：`filepath.Join(rootDir, sessionID+".jsonl")`，sessionID 仅 TrimSpace）；`:93-111`（`ResumeWithLimits` 同法拼 `path`，随后 `append` 以 `O_CREATE|O_WRONLY|O_APPEND` 写）；CLI `--resume-id` 入口 `cli.go:679`。由 Codex 提出、主审逐行核实。历史 `review-handoff-codex53-resume-cli-p0.md` 只覆盖 TTY/失败语义，未覆盖 sessionID 校验。
- **问题**：`lore console --resume-id "../../foo"` 读取（若存在且可解析）并追加 session 事件到 session 根之外的 `foo.jsonl`。
- **为什么重要**：读侧信息泄露、写侧越界追加；触发者为本地操作者，概率低但边界明确。
- **建议修复**：校验 sessionID 匹配生成格式（`^[a-z0-9-]+$`，拒分隔符/`..`/绝对路径），或解析后 require 仍在 session 根下。
- **需新增测试**：是。

### P2-3 · console · `--local-exec` 下 workspace_write 可改写 `.lore/config.json`
- **证据**：`internal/console/tool_runtime.go:712-758`（`resolveWorkspacePath` 只 block `VaultRootPath()` 与 `StateDirPath()`，**不** block `<workdir>/.lore/`）。由 Codex 提出、主审核实（config 在 `.lore` 而非 `state`）。
- **问题**：带 `--local-exec` 的模型可写 `.lore/config.json`，改写未来 LLM 端点/路径根等运行时行为。属 opt-in 不安全画像，故 P2。
- **建议修复**：把 `.lore/`（至少 `config.json`）纳入 `resolveWorkspacePath` 的 block；配置修改走带 schema 校验的专用命令。
- **需新增测试**：是。

### P2-4 · CLI · 位置参数 workdir 不设防，误建工作区
- **证据**：`internal/cli/cli.go:233-238`（`resolveWorkDir`：`if len(args)>0 && args[0]!="" { return filepath.Clean(args[0]) }`，`status`/`bootstrap`/`demo-p0a`/`demo-p0b` 均用）；仓库根现存 `--help/`、`--workdir/`（含 `state/`+`vault/`）即证据。
- **问题**：`lore bootstrap --workdir ./real` → workdir 变字面量 `--workdir`，`./real` 静默丢弃并在意外位置 bootstrap。与 flag 式命令（`tui --workdir`）不一致。
- **建议修复**：位置 workdir 拒绝 `-` 开头参数（或复用 `reorderFlagsBeforePositionals` 的 flagset 统一处理）；顺手删两个垃圾目录。
- **需新增测试**：是（`bootstrap --help` 应报错而非建目录）。

### P2-5 · orchestrator · 低治理直写无乐观并发/未串行化
- **证据**：`harness.go:679-717`（`WriteLowRiskNote` 不取 `applyMu`、`overwrite` 无 baseVersion 校验），对照 `harness.go:497-531`（`ApplyDraft` 取 `applyMu` + baseVersion hash 守卫）。
- **问题**：`vault_write_low` 与 `ApplyDraft` 或另一次 `vault_write_low` 并发写同路径时 last-writer-wins，静默丢弃并发编辑（原子 rename 保证无半损文件，但无冲突检测）。与 P1-2 同源。
- **建议修复**：随 P1-2 一并处理（覆盖需 baseVersion 或确认）。

### P3（简述）
- **P3-1 MCP 默认无鉴权**：`mcp/server.go:191-205`，未设 `LORE_MCP_API_KEY` 时 `validateProcessAuth` 直接放行。stdio 传输风险低，但提案入口可被本地进程灌草稿、读工具暴露 vault。建议文档明确"接非可信 agent 时必设 key"。（设了 key 时用 `subtle.ConstantTimeCompare` 常量时间比较，实现正确。）
- **P3-2 raw 中文字面量**：`internal/bootstrap/templates.go`、`internal/console/session.go` 各含 4 个 raw CJK 符文（已用 Python 核字节，**当前均合法 UTF-8、未损坏**）。但违反本库因历史 mojibake 事故（commit `2ac9974`）定下的"中文用 `\u` 转义"约定，是下次在 GB18030 机器编辑时的潜在回归点。建议转 `\u`。
- **P3-3 仓库卫生**：`--help/`、`--workdir/` 垃圾目录应删；`--help/` 未显式写进 `.gitignore`（虽被忽略）。

## 4. 文件系统与用户数据安全

- **读写路径**：核心 vault 写全部经 `WriteRelativeAtomic → ResolveWriteUnderRoot`，做了 lexical 根内校验 + `Lstat` 拒符号链接 + `EvalSymlinks` 实路径校验 + `nearestExistingParent`/`rejectUnsafeExistingParents` 逐级祖先符号链接检查——扎实。
- **是否限制在预期目录内**：绝大多数是；**例外是 process-sink**（P1-1）。
- **误删/覆盖/污染**：无删除路径；覆盖风险集中在 `vault_write_low overwrite=true`（P1-2/P2-5）。
- **dry-run/preview/diff/rollback**：草稿有 review（近似 preview）；低治理直写与本地 apply 无 diff/rollback；无自动备份（依赖用户自己的 Obsidian/git 历史）。
- **异常中断**：原子写 temp+fsync+rename 得当；`ApplyDraft` 写成功但状态持久化失败会落 critical Finding（`harness.go:548-557`），不静默发散。
- **路径穿越**：vault 层已挡；残留 process-sink（P1-1）与 sessionlog resume（P2-2）两处。
- **编码/大小写/跨平台**：`isPathUnderRoot` 用 `filepath.Rel`，`normalizeRelativePath` 统一斜杠，`sameRelPath` 用 `EqualFold`（Windows 合理），rune-safe 截断到位。

## 5. Obsidian / Markdown 语义边界

- **frontmatter**：不解析/不改写，markdown 当整体文本读写（`renderMarkdownNoteDraft` 只做 CRLF 归一 + 末尾换行）。保真但治理逻辑无 frontmatter 语义感知。当前定位可接受。
- **wiki/markdown 链接与 backlinks**：`FindBacklinks` 同时匹配 `[[base]]`/`[[base|`/`[[path]]` 与 `](url)`（含 `<url>`、`./`/`../` 归一），手写扫描、线性无 ReDoS。刻意不匹配裸 basename 避歧义。
- **附件路径**：`ExtractAttachmentRefs` 按扩展名分类，排除 http/data/mailto，`.md` 归非附件——只读，无写。二进制抽取未实现（已声明为 gap）。
- **重命名/移动后引用一致性**：未实现（也不声称做）。
- **非 ASCII 文件名**：正常路径工作；见 P3-2 源码内 raw CJK 隐患。
- **空/损坏/大文件**：空/损坏无专门处理；大文件/长行触发 P2-1。

## 6. 自动化 / AI / Agent 边界

- **是否未经确认改用户笔记**：**部分会**（P1-2）——本地 agent 的 `vault_write_low`、`draft_approve`/`draft_apply` 均模型可 inline 调用、无人类确认闸（唯 `shell_exec` 有）。persona 抽取则**不会**：`session.go:268/277` 的 `launchPersonaExtraction` 是 fire-and-forget，只调 `RecordPersonaCandidate` 落候选库，从不 `WriteLowRiskNote`/`ProposePersonaUpdate`（与 `persona/model.go:7` 注释"never auto-approves anything"一致，已核实）。
- **preview/confirm/rollback**：草稿有 review、shell 有 confirm；低治理直写与 apply 无 rollback。
- **区分建议/草稿/正式写入**：清晰——提案工具只建 `pending_review` 草稿；候选是更靠前的建议层，需显式 `draft` 提升。
- **变更来源记录**：到位——audit `Actor` 区分 `external_agent`/`operator`/`runtime`/`process_sink`。
- **外部 MCP 边界（强项）**：`mcp/server.go:183-189` 的 `callTool` 强制 `Surfaces().Has(SurfaceMCP)`，`vault_write_low`/draft 动作/local-exec 因不带 `SurfaceMCP` 位而无法从 MCP 泄漏——治理边界做成类型级不可违反。Codex 独立确认。

## 7. Contract / Config / Mock / Test 一致性

- **config 格式**：稳定，三层合并 + `Validate()`；已知坑（duration 用纳秒、短别名被拒、`api_key_ref` 未支持）在 README 与报错里写明，无漂移。
- **schema 与代码一致**：MCP 工具面（README "9 读 + 2 提案"）与 `builtin_readonly.go`/`builtin_proposal.go` 一致；5 个管理核心文档与 `bootstrap` 一致。
- **fixture 过期 / mock 漂移**：**无 mock 泄漏**——非测试代码里 `fake/mock/stub` 命中全是注释（描述测试注入缝），生产侧一律真实实现。DEVELOPMENT_STATUS 明确"Automated tests use fake providers"。
- **测试脆弱路径**：抽查未见硬编码本机绝对路径（用 `t.TempDir` 风格）。
- **文档命令有效性**：README/DEVELOPMENT_STATUS 命令与 `cli.go` dispatch 对得上。

## 8. 测试缺口（最该补）

现有测试覆盖良好（rune 截断、apply 并发、apply 状态失败、finding 状态机、persona 候选去重/认领、post-scan reconciliation 均有专测）。缺口：

1. **codex-jsonl / appserver 的 agentID 路径穿越**（P1-1）——关键不对称：`import_external_test.go:112` 已有"遍历写出文件、断言无一逃出根"的边界测试且 `:181-191` 断言 agentID 被消毒，但 `import_codex_test.go` **无** `--agent ../..` 用例。测试不对称正映射代码不对称。
2. **超长单行 .md 下的搜索/反链**（P2-1）。
3. **`--resume-id ../../x`**（P2-2）——现有 resume 测试只测 TTY/失败语义。
4. **`vault_write_low` 覆盖既有笔记 + 并发写同路径丢更新**（P1-2/P2-5）。
5. **CLI 位置参数防呆**（P2-4）——`bootstrap --help` 应报错。
6. **config 未知键 / 迁移**：`json.Unmarshal` 静默忽略未知键，建议加告警或拒绝的测试。
7. **损坏输入**：损坏 frontmatter / 非法 UTF-8 markdown / 空 vault / 大 vault 下读工具鲁棒性无专测。
8. **legacy store.json → SQLite 迁移失败/损坏**：`migrateLegacyJSONIfNeeded` 有 happy path，建议补"损坏 JSON 时事务回滚、不污染已有 DB"（`isEmpty()` across-all-tables 守卫值得专测）。

## 9. Codex 交叉审查

- **调用**：2 次 WSL `codex exec - --sandbox read-only --ephemeral`（Codex CLI 0.142.5）。任务 A = 文件写路径安全扫描；任务 B = mock 泄漏/文档漂移/测试缺口。
- **A（已返回、逐条核实）**：与主审结论**高度收敛**——独立定位 P1-1（call chain 与修复建议一致）、P1-2；补充 P2-2（`--resume-id` 穿越）与 P2-3（workspace_write 可改 `.lore/config.json`），两条均经主审回代码逐行验证后采纳。A 的"Guarded/Not Findings"（MCP 只读+提案、`WriteRelativeAtomic` 有守卫、workspace_write 挡 store/日志、无 `RemoveAll`）与主审判断一致，作交叉印证。
- **B（未产出）**：约 13 分钟零输出并伴 TTY 报错（疑似该次调用卡死），已终止以免占额度。B 本应覆盖的测试缺口/漂移/mock，主审已自行独立完成（第 6/7/8 节）。
- **结论**：无一条 Codex 结论未经主审核实即入本文；P2-2/P2-3 是 Codex 先点出、主审验证的实用增益。

## 10. 修复路线图

**P0**：无严格 P0。若偏保守，可把 P1-1 当 P0 对待。

**P1（强烈建议修）**
- **P1-1**：`import_codex.go` 的 `applyCodexJSONLIdentity` 覆盖后补 `codexjsonl.FinalizeTranscript(transcript)`；理想再让 process-sink 写入走 `ResolveWriteUnderRoot`。验证：新增 `import-codex-jsonl --agent "../../x"` 测试断言无文件逃出 sink 根 + `go test ./internal/app/... ./internal/domain/processsink/...`。
- **P1-2**：产品决策——`vault_write_low` 对既有文件的 `overwrite` 走 pending-confirm（复用 `PendingShellCommand`），或覆盖强制走草稿。验证：新增覆盖既有笔记的确认流测试 + 手动 TUI 跑一次覆盖。

**P2（后续优化）**
- **P2-1**：`vault/query.go:104,172` 各加 `scanner.Buffer(...)`。验证：长行 .md 搜索测试。
- **P2-2**：`sessionlog.Load`/`ResumeWithLimits` 校验 sessionID 格式。验证：`--resume-id ../../x` 被拒测试。
- **P2-3**：`resolveWorkspacePath` 把 `.lore/` 纳入 block。验证：workspace_write 写 `.lore/config.json` 被拒测试。
- **P2-4**：`resolveWorkDir` 拒 `-` 开头位置参数；删 `--help/`、`--workdir/`。验证：`bootstrap --help` 报错测试。
- **P2-5**：随 P1-2 处理。

**P3（可选）**
- P3-1 文档标注 MCP 默认无鉴权、接非可信 agent 设 `LORE_MCP_API_KEY`。
- P3-2 `bootstrap/templates.go`、`console/session.go` 的 raw CJK 转 `\u`（改前用 Python 核字节，勿信终端）。
- P3-3 清理仓库根垃圾目录、补 `.gitignore`。

> 提示：P1-1、P2-1 修复面极小且项目内已有可照抄范式；P1-2 是产品语义决策，建议先定方向。修复时注意 B 线/他线文件边界。

## 11. 建议验证命令

```powershell
# 基线（本次会话已跑，29 包全绿）
go test ./...
.\.tools\go\bin\go.exe test ./...

# 针对性回归（修复后重点跑）
go test ./internal/app/... ./internal/domain/processsink/... ./internal/vault/... ./internal/sessionlog/... ./internal/console/...

# 竞态检测
go test -race ./internal/orchestrator/... ./internal/runtime/... ./internal/store/...

# 冒烟 & 发布门
go run ./cmd/lore smoke p0 --workdir .\tmp\p0-smoke
go run ./cmd/lore tui --workdir .\tmp\p0-smoke --once "show current status"
.\scripts\release-gate.ps1
.\scripts\release-gate.ps1 -Full

# 复现 P2-4（安全；跑前后看仓库根多出的目录）
go run ./cmd/lore bootstrap --help
```
