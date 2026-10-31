# 定时测试：模型指纹检测（lm-detector 集成）

> 适用版本：sub2api-detecter（本 fork）新增功能。
> 检测原理与参考库来自 [LM Fingerpoint Detector](https://github.com/Ikaleio/lm-detector)：
> 让模型“随手写约 300 个 1–355 的整数”，用数字分布与参考库比对，给出最接近的候选模型。

## 1. 功能概览

账号管理的「定时测试」原本只做连通性测活。本功能新增：

| 能力 | 说明 |
|---|---|
| 指纹测试模式 | 计划级开关；开启后该计划按 cron 改为跑模型指纹检测，而非测活 |
| 检测参数 | 仅指纹模式可编辑（关闭时置灰），随计划保存 |
| 自动关闭 | **独立开关**。测试失败（断联/超时/HTTP 错误/指纹不一致）时关闭该账号的「调度」 |
| 自动恢复 | 测试成功时清理异常/限流状态；若账号是被本功能自动关闭的，则重新打开调度 |
| 可追溯结果 | 结果记录判定、检测模型 vs 期望模型、置信度、对账号调度执行的动作 |

一个账号可以建多个计划，分别用不同模型 + 不同 cron，检测不同上游模型的降智情况。

## 2. 使用步骤

1. 进入 管理后台 → 账号管理 → 目标账号 → 「定时测试」。
2. 新建计划：选择模型、填写 cron 表达式。
3. 打开「指纹测试」（test_mode=fingerprint）。
4. 按需调整检测参数（见 §4）。
5. 按需打开「自动关闭」/「自动恢复」。
6. 保存。到点后计划会按指纹模式执行，结果在计划行展开可见。

> 指纹测试关闭时 = 原有的测活行为，完全向后兼容；老计划升级后默认仍是测活。

## 3. 判定口径

| verdict | 界面 | status | 触发自动关闭 | 含义 |
|---|---|---|---|---|
| match | 一致（绿） | success | 否 | 检测到的 top1 与计划期望模型一致（归一化后相同，或开启同族容忍后同族） |
| mismatch | 不一致（红） | failed | 是 | 检测到的是另一个模型（疑似降智/换模） |
| invalid_reply | 回复异常（红） | failed | 是 | 数值序列不像模型生成（可能被劫持/注入/缓存） |
| error | 失败（红） | failed | 是 | 上游错误、超时、全部样本失败（等同断联） |
| unsupported | 不支持（灰） | failed | 否 | 账号类型不支持（OAuth/Gemini/Grok/Bedrock 等）或参考库不可用；不归责上游 |

- 模型名比较会做归一化：`.`/`_` → `-`、去供应商前缀、去日期后缀（如 `-20250929`），因此
  `claude-sonnet-4.5` 与 `claude-sonnet-4-5-20250929` 视为同一模型。
- 「同族视为一致」默认关闭；开启后同家族不同版本（如 sonnet-4.5 检测成 opus-4.6）也算一致，
  适合只关心“是不是这家厂”的场景。
- 多轮（rounds > 1）时：**全部轮次一致才算 match**；任意一轮不一致即 mismatch。

## 4. 检测参数（detector_config）

| 参数 | 默认 | 范围 | 说明 |
|---|---|---|---|
| API 协议 | auto | auto/responses/chat_completions/messages | auto 按账号平台与协议自动选择 |
| 每轮样本数 count | 3 | 1–3 | 一轮发多少个挑战；3 条有效样本才会给出校准置信度 |
| 并发 parallel | 3 | 1–3（≤count） | 同一轮内并发请求数 |
| 超时 timeout_sec | 120 | 1–600 | 单请求超时（流式为整体读取时限） |
| 轮次 rounds | 1 | 1–5 | 重复检测轮数；多轮更稳但更费额度 |
| 非流式 no_stream | 关 | 开/关 | 用普通 JSON 响应代替 SSE；部分中转对 JSON 支持更好 |
| 严格模式 strict | 关 | 开/关 | 要求 3/3 完整样本（自动锁定 count=3） |
| 推理强度 effort | 空 | 字母数字下划线连字符 | 透传 reasoning effort（如 high）；空=不发送 |
| 服务等级 service_tier | default | flex/default/fast/ultrafast | 透传上游服务等级 |
| 同族视为一致 allow_family_match | 关 | 开/关 | 见 §3 |

> 每次指纹测试的上游请求数 ≈ count × rounds（默认 3 次）。请按上游额度与频率合理设置 cron。

## 5. 自动关闭 / 自动恢复语义

- **自动关闭**与指纹开关互相独立：即使计划仍是测活模式，失败也会关闭该账号调度。
- 关闭动作：
  1. 在账号 `extra` 写入标记 `scheduled_test_auto_disable = {plan_id, at, reason}`；
  2. 设置账号 `schedulable=false`（立即从调度候选中移除）；
  3. 结果 `account_action=disabled`，界面显示「已关闭调度」。
- **自动恢复**（开启时）：测试成功 →
  1. 清理错误/限流/临时不可调度状态（沿用原有逻辑）；
  2. 若账号存在上述标记，则 `schedulable=true` 并清除标记，结果 `account_action=restored`；
  3. 若账号没有标记（例如被管理员手动关闭），**不会**擅自打开，避免误操作。
- 同一账号多个计划同一分钟到期时，runner 会按账号串行执行，避免并发关闭/恢复竞争。

## 6. 部署与配置

指纹参考库由 lm-detector Go SDK 懒加载（首次使用约 1 秒，常驻内存约 9MB）：

| 环境变量 | 说明 |
|---|---|
| `FINGERPRINT_ENABLED` | 默认 `true`；`false` 时指纹计划一律返回 unsupported |
| `FINGERPRINT_DATA_DIR` | 本地参考库目录（含 `unified_bank.json` + `shared_detector.bin`），优先使用、完全离线 |
| `FINGERPRINT_DATA_SOURCES` | 逗号分隔镜像 base URL 列表，覆盖 SDK 默认（lm.ikale.io / npm 镜像） |
| `FINGERPRINT_CACHE_FILE` | 参考库缓存文件路径，默认 OS 缓存目录下的 `lmfpd/data.bin` |

离线部署示例（把 lm-detector 仓库的 `data/` 挂载进容器）：

```yaml
environment:
  FINGERPRINT_DATA_DIR: /app/data/lmfpd
volumes:
  - ./lmfpd-data:/app/data/lmfpd:ro
```

目录内需要：`unified_bank.json`（约 13.9MB）与 `shared_detector.bin`（约 4.7MB）。

## 7. 已知限制与风险

- **参考库是封闭集合**：不在库里的模型也会得到一个“最像”的候选；置信度只在库内模型之间比较，
  不是身份证明。建议结合 top5 排名、多轮结果与业务证据判断。
- v1 仅支持 **API Key 类型上游**（Anthropic / OpenAI / 多协议国产供应商）；OAuth、Gemini、Grok、
  Bedrock 账号返回 unsupported，不会触发自动关闭。
- Go SDK 不提供分词器探测，本功能也不做；`rounds` 用于降低误判。
- 检测本身会产生上游费用；`strict`、大 `count`、多 `rounds` 都会增加请求量。
- 自动关闭会影响线上可用性：建议先只开指纹测试观察一段时间（不勾自动关闭），确认判定稳定后再开启。

## 8. 排障

| 现象 | 排查 |
|---|---|
| 结果 unsupported，提示参考库加载失败 | 检查 `FINGERPRINT_DATA_DIR` 文件是否齐全，或服务器能否访问数据镜像 |
| 结果 error，提示上游 4xx/5xx | 看 detail 中每轮样本的 error 与 HTTP 状态；确认账号 base_url/协议 |
| 参数保存 400 | 检查 count/parallel/timeout/rounds 范围、strict 是否配 count=3、api/service_tier 白名单 |
| 期望模型一直 mismatch | 打开「同族视为一致」或手动确认上游实际模型；可通过 detail 的 results 查看 top5 |
| 自动关闭后账号不恢复 | 需开启「自动恢复」；且账号必须存在本功能写入的标记（管理员手动关闭不会被覆盖） |
## 9. 批量模型指纹测试（账号模型白名单）

> 2026-10-30 新增。入口在「编辑账号 → 模型限制 → 模型白名单」，用于一次性检测白名单里的**所有模型**，
> 不需要为每个模型单独建定时计划。

### 9.1 使用方式

1. 打开 **账号管理 → 编辑账号 → 模型限制 → 模型白名单**；
2. 在白名单操作区（「同步上游模型 / 清除所有模型」旁）点击 **「一键测模型指纹」**；
3. 在弹窗里确认/调整参数，点击 **「开始测试」**；
4. 弹窗展示进度与逐模型结果；关闭弹窗后任务在后台继续，再次打开可回看最近一次任务；
5. 运行中可 **「取消测试」**；取消后未开始的模型标记为已取消，已产生的结果保留。

按钮使用**当前表单里的白名单**（包含尚未保存的修改），不依赖是否已保存账号。
通配符条目（如 `claude-opus-*`）无法测试具体指纹，会被自动跳过并在弹窗中提示。

### 9.2 参数

| 参数 | 说明 |
|---|---|
| 限制并发（模型） | 同时测试的模型数，1–3，默认 2 |
| API 协议 / 每轮样本数 / 并发 / 超时 / 轮次 / 非流式 / 严格 / 推理强度 / 服务等级 / 同族视为一致 | 与「定时测试 → 指纹检测参数」完全一致，见第 4 节 |
| 预估请求数 | 模型数 × 样本数 × 轮次；单任务上限 **600** 次上游请求 |

判定口径与定时指纹测试相同：

- `match`：top1 与期望模型同族（开启「同族视为一致」时按族比较）；
- `mismatch`：检测到其它模型（通常意味着降智/换模）；
- `unsupported`：账号类型不支持（OAuth/Gemini/Grok/Bedrock 等）；
- `error` / `invalid_reply`：上游错误或回答格式无效，对应结果状态 failed。

> 批量测试**只报告结果**：不会自动关闭账号调度、不会修改白名单、不影响任何计划。
> 需要自动处置请使用「定时测试」的指纹模式 + 自动关闭/自动恢复。

### 9.3 一致性与限制

- 同一账号**同时只允许一个**批量任务；重复点击/多标签页并发创建会返回 409 并展示已有任务。
- 前端每次点击生成 `Idempotency-Key`，相同 key 的重试返回同一个任务（防双击重复扣费）。
- 结果按白名单顺序保存（`position`）；每个模型最多保留 **16KB** 检测明细，超出会裁剪并标记 `detail_truncated`。
- 每个账号保留最近 **20** 个批量任务（更早的自动清理）。
- v1 的任务在**单个服务进程内执行**：多实例部署时创建约束由数据库保证，但任务不会跨实例迁移；
  服务重启会把未完成任务标记为 failed（`terminal_reason=restart`）。
- 后端接口（需要管理员 token，写操作需已完成合规确认）：

```bash
# 创建（models 为当前白名单快照；detector_config 可省略）
curl -s -X POST "http://127.0.0.1:18080/api/v1/admin/accounts/<account_id>/model-fingerprint-runs" \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"models":["claude-haiku-4-5","claude-sonnet-4-6"],"concurrency":2,
       "detector_config":{"api":"auto","count":3,"parallel":3,"timeout_sec":120,"rounds":1,
                          "no_stream":false,"strict":false,"effort":"","service_tier":"default",
                          "allow_family_match":false}}'

# 查询任务与结果
curl -s "http://127.0.0.1:18080/api/v1/admin/accounts/<account_id>/model-fingerprint-runs?limit=5" \
  -H "Authorization: Bearer <token>"
curl -s "http://127.0.0.1:18080/api/v1/admin/accounts/<account_id>/model-fingerprint-runs/<run_id>" \
  -H "Authorization: Bearer <token>"

# 取消
curl -s -X POST "http://127.0.0.1:18080/api/v1/admin/accounts/<account_id>/model-fingerprint-runs/<run_id>/cancel" \
  -H "Authorization: Bearer <token>"
```

### 9.4 数据表（迁移 250）

- `account_model_fingerprint_runs`：任务（状态、参数快照、模型快照、计数、预算、终止原因、幂等键）；
- `account_model_fingerprint_results`：逐模型结果（`position`、判定、检测模型/族、置信度、耗时、detail JSON）。

升级说明：正常重启服务即可自动执行迁移 250；无需手工操作，也不影响既有定时测试数据。

### 9.5 排障

| 现象 | 排查 |
|---|---|
| 按钮置灰 | 白名单为空（空表示允许全部模型，无法枚举）；或账号未保存（新建账号） |
| 400 `MODEL_FINGERPRINT_BUDGET_EXCEEDED` | 模型数 × 样本数 × 轮次超过 600；减少模型/样本/轮次 |
| 400 `MODEL_FINGERPRINT_INVALID_MODELS` | 白名单含通配符 `*`；请改用精确模型 ID（前端已自动跳过通配符） |
| 409 `MODEL_FINGERPRINT_RUN_ACTIVE` | 该账号已有进行中的批量任务；弹窗会自动加载该任务 |
| 全部结果 unsupported | 账号类型不支持指纹检测（v1 仅 API Key 上游） |
| 任务状态 failed + `terminal_reason=restart` | 服务重启导致任务中断，重新发起即可 |
| 结果数据量大/加载慢 | detail 已裁剪到 16KB；列表接口只返回任务摘要，详情单独加载 |
## 10. 自动别名匹配（2026-10-31 新增）

### 10.1 背景与规则

指纹检测把「上游模型 ID」与 lm-detector 参考库标签比较。供应商常给同一模型起不同 ID：
例如上游 ID 是 `6.1sol`，参考库标签是 `gpt-6-astra`，其展示名是 `gpt-6-astra/6.1-sol`。
早期只做精确比较，这类情况会误判「不一致」。

现在改为**自动别名匹配**（无需手工配置）：

1. 参考库的 `display_name` 按 `/`、`,`、`;` 切出别名段（如 `gpt-6-astra/6.1-sol` → `gpt-6-astra`、`6.1-sol`）；
2. 规范化时插入「数字→字母」边界：`6.1sol` → `6-1-sol`，从而与 `6.1-sol` 别名段相等；
3. 只做**整段精确相等**，不做 contains/前缀/编辑距离匹配；
4. 期望与检测都必须解析到**唯一**参考模型才可判一致；别名指向多个模型、或检测的 prediction 与展示名冲突时，
   结果为 `error`（不会 match，也不会自动停用账号）；
5. 判定优先级：精确 → 别名 →（手动开启时）同族。

### 10.2 安全规则

- 指纹模式下只有明确的 `mismatch` 才会触发「自动关闭」；上游错误、无效回复、无法评分、不支持账号、歧义结果
  都不会关闭调度（避免把上游抖动或参考库歧义当成降智）。
- 结果列表判定徽标旁会提示「别名匹配」/「同族匹配」；
- 结果明细（展开「检测明细」）包含：
  `expectation.expected / resolved_model / matched_via / matching_mode / bank_sha256`，
  用于确认本次是否命中参考库别名、是否启用了别名索引。

### 10.3 排障

| 现象 | 处理 |
|---|---|
| 同一模型仍判 mismatch | 展开检测明细：若 `matched_via` 为空且 `resolved_model` 为空，说明该上游 ID 与参考库没有可识别的别名关系；这是参考库身份判定结果，不是检测故障 |
| 结果里 matching_mode=legacy | 参考库未加载（`FINGERPRINT_DATA_DIR` 缺失），别名匹配不可用；补齐参考库目录后重启 |
| 出现「检测结果包含多个参考模型/别名有歧义」 | 参考库存在歧义别名；核对上游实际模型后按真实身份判断，系统不会强行匹配 |
| `6.1sol` 检测为 `gpt-6-astra/6.1-sol` 却未匹配 | 确认 `matched_via`；正常情况下应为 `alias`。若为 legacy，检查参考库挂载 |
