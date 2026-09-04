# leeway 调研报告（2026-09-02）

> 面向**真实订阅使用**的完整调研汇总：各家 AI 编程 provider 的额度查询方案、登录/多账号机制、
> 现状与缺口、待处理点与预期方案。证据分级：`实测`=本机跑过；`逆向`=从官方 CLI bundle 确认；
> `社区`=同类工具/issue 佐证；`推断`=未直接验证。
> 同类用量工具怎么拿登录态、怎么刷新、哪些余量是官方数：见 [COMPETITORS.md](COMPETITORS.md)（2026-09-03）。
> 下列「待处理」里 B1/B2/C1/C2 已在后续实现落地（Credits = limit−includedSpend；Grok REST；commandcode provider；文档已补），本文保留当时记录。

## 0. TL;DR

1. **额度查询三档结论**：有官方公开 API 的（DeepSeek、OpenCode Go）已是最优；无公开 API 的
   （Cursor、Grok）分两派——Cursor 走官方 dashboard 内部 RPC（bearer 可用，正确）、
   Grok 走 grpc-web + protobuf 启发式（**应升级到 cli-chat-proxy REST**，能修掉 14.0%→13% 的
   精度误差与"窗口类型靠猜"问题）。
2. **登录机制**：Cursor 浏览器授权 = 纯 PKCE + 服务端 uuid 绑定 + CLI 轮询，**无本地回调**。
   自实现逆向该私有协议已被用户否决（安全风险）。已重构为**委托官方 `cursor-agent login`**
   （`AGENT_CLI_CREDENTIAL_STORE=file` + 私有 `HOME`），leeway 只读官方写出的 `auth.json`——
   与 grok 委托模式同构，全代码已落地、测试绿、已部署。
3. **多账号模型**：每账号一个私有 HOME/auth.json，与官方 CLI/keychain 状态彻底解耦，
   切号只换 `--account`，无需 logout/login。
4. **待处理点**：2 项需用户执行的登录迁移 + 1 个小显示 bug + 1 处文档同步 +
   2 项功能增强（Grok REST 升级、Command Code GOAT 接入），详见 §7。

---

## 1. 用户真实订阅 vs leeway 现状

| 订阅 | 账号数 | leeway 认证现状 | 额度查询现状 | 缺口 |
|---|---|---|---|---|
| DeepSeek | 1 | 未配 key（not_configured） | 官方 API，最优 | 配一把 key |
| OpenCode Go | 3（bxs/github/gmail） | 3 个订阅 key 全部 ok | 官方网关，最优 | 无 |
| Cursor | 2 | **只跑通 1 个**（default=keychain 兜底） | api2 RPC，正确但有显示 bug | 多账号登录迁移 + remaining bug |
| Grok | 2 | default 仍在 `~/.grok (shared)` | grpc-web 启发式，有精度误差 | 私有化登录 + REST 升级 |
| Command Code GOAT | 计划多号 | 已接入（API key） | — | 新 provider |
| （顺带支持）Codex / Kimi | — | CLI 会话复用 | — | 用户未订阅，仅工具覆盖 |

---

## 2. 各家额度端点调研（实测/逆向证据）

### 2.1 DeepSeek —— 已最优（官方公开 API）`实测`

| 项 | 值 |
|---|---|
| 端点 | `GET https://api.deepseek.com/user/balance`（Bearer API key） |
| 窗口 | 余额（¥） |
| 坑 | `platform.deepseek.com` 有 WAF 拦 API key，**余额必须走 api. 域** |
| 结论 | 官方公开 API，无更优解。本机未配 key 属用户侧待办 |

### 2.2 OpenCode Go —— 已最优（官方网关，天然多账号）`实测`

| 项 | 值 |
|---|---|
| 端点 | `GET https://opencode.ai/zen/go/v1/usage`（Bearer 订阅 key） |
| 窗口 | `{usage:{rolling,weekly,monthly}}` 三档 |
| 多账号 | 每把订阅 key 一个账号（`accounts[].apiKey`），3 号全 ok |
| 结论 | 官方 OpenAI 兼容网关，社区（CodexBar 等）同款，已最优 |

### 2.3 Cursor —— 路线正确，两处待修 `实测`

| 候选路线 | 结果 | 证据 |
|---|---|---|
| **api2 RPC**：`POST api2.cursor.sh/aiserver.v1.DashboardService/GetCurrentPeriodUsage`，Bearer（keychain `cursor-access-token`/`cursor-user` 或私有 auth.json） | ✅ 可用，返回 planUsage(auto/api/total percent、includedSpend、bonusSpend、limit) + billingCycleStart/End | 本机多账号实测 |
| usage-summary cookie 路线（CodexBar 主用） | ❌ 需 `WorkosCursorSessionToken` cookie，bearer 实测 401 | 实测 |
| Cursor IDE SQLite 路线（CodexBar 备选） | ❌ 本机无 `state.vscdb`（未装/未登录 Cursor IDE） | 实测 |
| **已知 bug** | 响应**无 `remaining` 字段**，leeway 的 Credits 现金条恒显 `0.00 / 40000.00`，误导 | 实测 |

### 2.4 Grok —— 应升级到 CLI-proxy REST `实测`

| 路线 | 结果 |
|---|---|
| **（推荐）** `GET https://cli-chat-proxy.grok.com/v1/billing?format=credits`，头 `Authorization: Bearer <auth.json key>` + `x-xai-token-auth: xai-grok-cli` | 干净 JSON：`config.creditUsagePercent`、`currentPeriod{type,start,end}`、`productUsage[]{product,usagePercent}`、`prepaidBalance`、`billingPeriodEnd`；另有 `GET /v1/settings` 拿 plan 名 |
| 现状：`grok.com/grok_api_v2.GrokBuildBilling/GetGrokCreditsConfig` grpc-web + protobuf 启发式 | ⚠️ 14.0% 被算成 13%（fixed32 启发式）；窗口类型（Weekly/Monthly）靠"重置距离猜"；无公开 schema，脆弱 |

### 2.5 Command Code GOAT —— 待接入（新 provider 候选）`实测`

| 端点 | 结果 |
|---|---|
| `GET api.commandcode.ai/alpha/whoami` | 存在，401 = 需真 key |
| `GET api.commandcode.ai/alpha/usage/summary` | 存在，401 |
| `GET api.commandcode.ai/alpha/billing/credits` / `subscriptions` | 存在，401 |
| `GET api.commandcode.ai/provider/v1/models` | 200（公开） |
| key 形态 | `user_...`，commandcode.ai/studio 签发，CLI 与 API 共用；纯 API-key → 多账号天然支持（同 opencode-go 模式） |
| GOAT 档位 | 5h $14 / 周 $35 / 月 $70 |

---

## 3. Cursor 登录机制逆向结论（谁发起 / 谁写入 / 谁回调）

来源：`cursor-agent` 官方 bundle（2026.08.31-4057e58）`index.js` + cli-credentials/dist。`逆向`

### 3.1 流程（无本地回调）

```
CLI（本地）: 生成 verifier / sha256(verifier)=challenge / uuid
   → 浏览器开 cursor.com/loginDeepControl?challenge&uuid&mode=login&redirectTarget=cli
   → 用户在浏览器授权；服务端把 token 绑到 uuid（没有本地端口回调！）
CLI: 轮询 GET api2.cursor.sh/auth/poll?uuid&verifier
     404 = 等待中（退避 1s×1.2^n，上限 10s，150 次≈20min 超时）
     200 = {accessToken, refreshToken}
     403 = 用户拒绝
```

- `redirectTarget=cli` 只是页面文案参数，**无端口回调**。
- poll 请求**无隐式 headers**（仅设 `CURSOR_AGENT_CLI_LOCAL_MODE=true` 才加
  `local-cli-mode:true`）。→ 早前 leeway 自实现轮询"格式/头字节级一致仍收不到 token"的根因
  **不是格式问题，而是自实现私有协议这条路本身不该走**（脆弱 + 无保障 + 合规风险）。

### 3.2 官方凭据双模式（多账号隔离的钥匙）

| `AGENT_CLI_CREDENTIAL_STORE` | 落点 |
|---|---|
| `default`（缺省） | macOS keychain：account=`cursor-user`，service=`cursor-access-token`/`cursor-refresh-token` |
| `file` | `~/.cursor/auth.json`（**homedir 读 $HOME → 私有 HOME 即隔离**），0600，`{accessToken,refreshToken,apiKey}` |
| `memory` | 不落盘 |

### 3.3 Token 生命周期

- **60 天 session JWT，无 refresh 端点**（官方到期也是重登；keychain 里 refresh-token 与
  access-token 同款 HS256 session，exp 相同）。
- 官方 CLI 另有直通参数（leeway 用不到，备用）：`--auth-token`/`CURSOR_AUTH_TOKEN`、
  `--api-key`/`CURSOR_API_KEY`、`--endpoint`。

### 3.4 安全结论（已落地）

> 原则：**官方公开 API 优先；无公开 API 时委托官方 CLI 登录 + 只读官方落盘文件；
> 不逆向重放私有协议。**

- 逆向仅用于"搞清楚机制"，不作为实现手段。
- leeway 现实现：`leeway auth login --provider cursor` → 跑官方 `cursor-agent login`
  （env `AGENT_CLI_CREDENTIAL_STORE=file` + `HOME=<sessions>/cursor[-acct]`）→ 官方自己完成
  浏览器授权并写 `<home>/.cursor/auth.json` → leeway 读回校验 + 显示 JWT 到期。
- `auth logout --provider cursor` 只删私有 HOME，**keychain 官方会话不碰**。

---

## 4. 多账号模型（与官方状态解耦，2026-09-02 确立）

| provider | 每账号凭据载体 | 登录命令 | token 生命周期 | 切号 |
|---|---|---|---|---|
| cursor | 私有 `HOME` 下的 `.cursor/auth.json`（0600） | `leeway auth login --provider cursor [--account A]` | 60d JWT，无 refresh，过期报错提示重登 | 换 `--account` |
| grok | 私有 `GROK_HOME` 的 `auth.json`，自动绑 `accounts[].authFile` | `leeway auth login --provider grok [--account A]` | OIDC 自动 refresh 写回 | 换 `--account` |
| opencode-go | `accounts[].apiKey`（订阅 key） | `config set-api-key --account` | key 不过期 | 换 `--account` |
| deepseek | `apiKeys` / env | `config set-api-key` | key 不过期 | — |

- fetch 层 token 解析（cursor）：env `CURSOR_ACCESS_TOKEN` > account apiKey > **私有
  auth.json（有即用、空/过期报错，不回退 keychain 防账号混用）** > keychain（仅兜底）。
- fetch 层 not_configured 拦截已计入 `acc.AuthFile`（修过 bug + 回归测试）。
- `leeway auth status` 提供多账号台账（凭据来源 + 到期时间），cursor 区分
  私有 auth.json vs keychain，grok 区分私有 vs `~/.grok (shared)`。

---

## 5. 社区同类工具定位（非竞品对照）

| 工具 | 语言/形态 | 定位 | 与 leeway 差异 |
|---|---|---|---|
| **CodexBar**（steipete） | Swift，菜单栏 + CLI | 29+ provider 用量仪表盘 | leeway 的参照物（砍掉 UI 做纯 CLI/JSON） |
| **ccusage**（ryoppippi） | Rust | 本地 JSONL 成本分析（"已花多少"） | 互补：leeway 答"还剩多少" |
| **AgentSubscriptionUsage** | Rust，CLI | 统一多 provider 查询 | cursor 用 `~/.config/cursor/auth.json` → 同款 api2 RPC |
| **cclimits** | Python | statusline 用 one-liner | 带缓存，轻量 |
| **tokscale** | Rust/TUI | Grok/Kimi/Codex 多账号 | TUI 形态 |
| **pi-quota-monitoring** | pi 扩展 | commandcode + opencode-go | 证明 commandcode 纯 API-key 多账号可行 |

---

## 6. 已处理点（重构历史，供追溯）

1. ~~自实现 Cursor PKCE + poll~~ → **已退役删除**（用户安全否决；`internal/auth/login.go`、
   `session.go` 全清，uuid 依赖移除）。
2. 新增 `internal/auth/cursorfile.go`：官方 auth.json 读写（CursorHome/CursorAuthFile/
   LoadCursorAuth/RemoveCursorHome），含路径逃逸清洗（测试 6 下划线断言）。
3. `cli/auth.go`：loginCursor/loginGrok 全委托官方 CLI；logout 语义修正（cursor 删私有目录、
   grok 删私有 GROK_HOME、api-key provider 指向 config）。
4. `cursor/fetcher.go`：resolveToken 私有 auth.json 优先 + **不回退 keychain 防混用**。
5. 测试：14 包全绿（含 TestCursorHomeIsolation、TestResolveTokenFromCursorAuth、
   TestCursorAuthBlocksKeychainFallback 等）；已部署 `~/go/bin/leeway`。

---

## 7. 需要处理的点 + 预期方案（按优先级）

### A. 用户操作（非代码，各 ~2 分钟）

| # | 事项 | 现状 | 预期方案 | 验收 |
|---|---|---|---|---|
| A1 | **Cursor 两账号真实登录迁移** | default=keychain 兜底、无私有 auth.json；第二个号未配 | `leeway auth login --provider cursor`（主号，弹浏览器授权）→ `leeway auth login --provider cursor --account <二>`（如 work） | `leeway auth status` 两行 cursor 私有 auth.json + 各自到期；`usage --provider cursor --account work` 出数据 |
| A2 | **Grok default 私有化**（可选） | default=`~/.grok (shared)`（与官方共享） | 需要独立账号台账时跑 `leeway auth login --provider grok` 重新登录到私有目录；若只求"能查"，shared 也 OK（OIDC 自动续） | `auth status` grok default 显示私有 authFile |

> 前提：`cursor-agent` 与 `grok` CLI 在 PATH（leeway 找不到会明确提示安装命令）。

### B. 代码小修（P1，共约 1 小时）

| # | 事项 | 根因/证据 | 预期方案 | 改动面 |
|---|---|---|---|---|
| B1 | **Cursor Credits 显示 bug（已落地）** | GetCurrentPeriodUsage 响应无 `remaining`，`toModel` 的 Credits 条恒 `0.00 / 40000.00` | `remaining = limit − includedSpend`（limit>0 时），语义变成"周期内可花额度"；或直接去掉现金条只留百分比。**推荐前者** | `cursor/fetcher.go toModel` 约 5 行 + 1 个单测 |
| B2 | **文档/注释同步（已落地）** | `cursor/provider.go` 头注释仍写"env>apiKey>keychain"，漏私有 auth.json；README 未列 cursor provider 与 `auth login/logout/status`；DESIGN/PROVIDER-FETCH 均未收 cursor + 委托模型 | 同步 provider.go 注释；README 补 Cursor 行 + auth 命令 + §4 引用；DESIGN/PROVIDER-FETCH 各补一小节 | 3 个 md + 1 段注释 |

### C. 功能增强（P2）

| # | 事项 | 现状/差距 | 预期方案 | 改动面 | 验收 |
|---|---|---|---|---|---|
| C1 | **Grok 升级 CLI-proxy REST（已落地）** | 启发式 protobuf：14.0% 算成 13%、窗口类型靠猜、无公开 schema | fetch 优先 `GET cli-chat-proxy.grok.com/v1/billing?format=credits`（Bearer auth.json key + `x-xai-token-auth: xai-grok-cli`）；解析 `creditUsagePercent`/`productUsage`/`billingPeriodEnd`；**保留 grpc-web 为 fallback**（REST 4xx/5xx 时）；`/v1/settings` 补 plan 名 | 新增 `provider/grok/rest*.go`（fetcher+parser）+ stub 主 label 修正 + 单测，约 200 行 | 本机 REST 输出 vs 现启发式对照，百分比一致且带准确周期类型 |
| C2 | **接入 Command Code GOAT（已落地）** | 已接入（API key） | 新 provider `commandcode`：`/alpha/usage/summary`（主）+ `/alpha/billing/credits`（辅）；认证 `accounts[].apiKey`（`user_...`），完全同 opencode-go 订阅模式；AuthSpec 声明 + registry 注册 + 文档 + 测试 | `internal/provider/commandcode/` 3~4 文件约 250 行 | 用户给一把 key 后 `leeway usage --provider commandcode` 出 5h/周/月三档 |

### D. 观察项（不立即做）

- Cursor 60d token 到期是官方行为，leeway 已做过期提示；**续期 = 重跑 A1**（无法自动化）。
- Grok REST 升级前先复测一次现 auth.json bearer 的可用性（防 token 类型漂移）。
- Command Code 若有官方公开 pricing/plan API，接入前再核一次档位命名（5h/周/月）。

---

## 8. 建议执行顺序

1. **B1+B2**（小改，我先做，30–60 分钟可交付）→ 顺带把 README 的 Cursor 支持补齐。
2. **A1+A2**（你操作，各 2 分钟；B 完成后跑，auth status 台账即全绿）。
3. **C1**（Grok REST，收益明确：精度 + 周期类型，半天内）。
4. **C2**（Command Code GOAT，等你提供一把 key 后做端到端验收）。

> 原则再次强调：凡涉及"浏览器授权拿 token"一律委托官方 CLI，leeway 只读落盘文件，
> 不再出现自实现 OAuth/PKCE 类代码（AGENTS.md 反过度设计 + 安全双重要求）。
