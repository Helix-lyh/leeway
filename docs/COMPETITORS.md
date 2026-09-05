# 竞品调研：用量仪表盘 vs leeway

> 日期：2026-09-03。对照公开仓库与文档。
> 问题：别人怎么拿登录态、怎么存、怎么刷新、怎么显示余量；leeway 该学什么。
> 相关：[RESEARCH.md](RESEARCH.md)（各家官方端点）、[PROVIDER-FETCH.md](PROVIDER-FETCH.md)（leeway 实现）。

证据：`源码`=读过仓库；`文档`=README / vendor-endpoints；`社区`=同类工具互相引用。

---

## 0. 一句话

没有工具能把 Cursor / Claude / Codex / OpenCode Go / Command Code GOAT / DeepSeek 现金 / Grok 订阅池的**官方余量**一次打通。

分两拨：

| 档 | 代表 | 真正回答 |
|---|---|---|
| 官方余量 | OpenUsage.ai、CrossUsage、ai-usagebar、UsageAtlas（Claude/Codex/Cursor） | 还剩百分之几、几点重置、账户还剩多少钱 |
| 本地估算 | OpenUsage.sh、UsageAtlas 的 OpenCode、AIUsage、Token Monitor、AIUsageTracker 的 `opencode stats` | 本地日志烧了多少 token / 估多少钱 |

leeway 只做上一档。Cursor / OpenCode Go / DeepSeek / Grok / Command Code 这条线上，官方余量最接近打满的是 **leeway + ai-usagebar**，不是 OpenUsage。

---

## 1. 看过哪些项目

| 项目 | 仓库 / 站点 | 形态 | 源码版本 |
|---|---|---|---|
| OpenUsage.ai | [robinebers/openusage](https://github.com/robinebers/openusage) · [openusage.ai](https://www.openusage.ai/) | macOS 菜单栏（Swift 重写） | 公开仓库 |
| CrossUsage | [barramee27/crossusage](https://github.com/barramee27/crossusage) · [crossusage.dev](https://crossusage.dev/) | Linux/Windows 托盘（旧 OpenUsage 的 Tauri 叉） | 公开仓库 |
| OpenUsage.sh | [janekbaraniewski/openusage](https://github.com/janekbaraniewski/openusage) · [openusage.sh](https://openusage.sh/) | Go 终端仪表盘 + daemon，36 家 | `524b4a6` |
| UsageAtlas | [SRX9/UsageAtlas](https://github.com/SRX9/UsageAtlas) · [usageatlas.com](https://usageatlas.com/) | 桌面端，Codex/Claude/Cursor/OpenCode | `41ee3cf` |
| ai-usagebar | [akitaonrails/ai-usagebar](https://github.com/akitaonrails/ai-usagebar) | Rust 状态栏 / TUI；GOAT + OpenCode Go | `v1.10.0` |
| AIUsageTracker | [rygel/AIUsageTracker](https://github.com/rygel/AIUsageTracker) | Windows 托盘 + `act` CLI | 公开仓库 |
| AIUsage | [juliantanx/aiusage](https://github.com/juliantanx/aiusage) | 本地 JSONL/SQLite 账单 | 文档 + `quota.ts` |
| Token Monitor | Token Monitor / iFence | 本地日志聚合 | 调研时对照 |

OpenUsage.ai 与 OpenUsage.sh **不是同一个产品**。前者盯订阅窗口，后者盯本机花费。CrossUsage 跟的是前者的旧插件树，Command Code / DeepSeek 还在那边。

---

## 2. 覆盖：你关心的套餐

| 套餐 | leeway | OpenUsage.ai | OpenUsage.sh | UsageAtlas | ai-usagebar | AIUsageTracker |
|---|---|---|---|---|---|---|
| Cursor 官方窗口 | 有，多账号隔离 | 有，单登录，会写回 token | 有，刮 IDE sqlite | 有，只读 IDE sqlite | 有，IDE sqlite 优先 | 无 |
| OpenCode Go 5h/周/月 | **官方** `/zen/go/v1/usage` | 有 | **无**（月度 USD） | **假**：本地花费 ÷ $12/$30/$60 | 有（只认 key） | **无**（PAYG credits / `opencode stats`） |
| Command Code GOAT | 有（粘贴 key） | 无 | 无 | 无 | **有**（读官方 CLI 文件 + 美元窗） | 无 |
| DeepSeek 现金 | 有 | 无 | 有 | 无 | 有 | 有 |
| Grok 订阅池 | **有**（REST + grpc-web） | 有（REST only） | **错表**（xAI API 预付费） | 无 | 有（SuperGrok） | 无 |
| Claude 5h/周 | 无 | 有 | 有 | 有 | 有 | 有 |
| Codex session/周 | 有（读 `auth.json`） | 有（自己 refresh 写回） | 有 + CLI RPC | **有且不碰 token** | 有（写回） | 有 |

---

## 3. 登录：获取 / 保存 / 刷新

行业默认是「机器上已经登录过，我就去读」。几乎没人再自做一遍浏览器 PKCE。差别在读哪里、拷不拷走、过期谁刷新。

### 3.1 获取

| | leeway | 别人常见做法 |
|---|---|---|
| 哲学 | 声明式：env → 命名账号 → 官方文件。浏览器登录委托官方 CLI | 零配置扫 IDE sqlite / keychain / `auth.json` |
| Cursor | `cursor-agent login` + 私有 HOME 的 `.cursor/auth.json`；无私有文件才读 keychain | 多数读 IDE `state.vscdb` 的 `cursorAuth/accessToken`（一台电脑一个登录） |
| Codex | 读 `~/.codex/auth.json` | UsageAtlas **不读文件**，拉起 `codex app-server`；其余读文件 |
| Grok | 私有 `GROK_HOME` 或 `~/.grok/auth.json` | OpenUsage 只认 `~/.grok`，不理 `GROK_HOME` |
| OpenCode Go | `accounts[].apiKey` | 从 `~/.local/share/opencode/auth.json` 抠 key；UsageAtlas 只当「有没有 Go」的布尔 |
| Command Code | env / 粘贴 key | ai-usagebar：env → `~/.commandcode/auth.json` → `~/.pi/agent/auth.json` |
| Claude | 无 | 钥匙串 `Claude Code-credentials` 或 `~/.claude/.credentials.json`。UsageAtlas **漏了钥匙串** |

### 3.2 保存

| | 做法 | 评价 |
|---|---|---|
| leeway | 官方文件留在官方位置（0600）；自己只存 API key 和 cookie 缓存 | 干净 |
| OpenUsage.ai | 不建 vault，直接改官方 sqlite/keychain | 会话所有权被它拿走 |
| CrossUsage | 官方文件 + AES-GCM 账号库（可粘贴 access/refresh） | 第二套 OAuth，易和官方 CLI 打架 |
| OpenUsage.sh | 发现到的 key/cookie **拷进** `~/.config/openusage/credentials.json` | 密钥双份 |
| UsageAtlas | 不存 provider 密钥 | 最好 |
| ai-usagebar | API key 可选写 `config.toml` 0600；OAuth 不拷 | 好 |
| AIUsageTracker | 扫描结果写进 `~/.opencode/auth.json` | 污染 OpenCode 自己的文件 |

### 3.3 刷新

| 家 | leeway | 别人 | 该不该学写回 |
|---|---|---|---|
| Cursor JWT（约 60 天） | 过期提示重登 | OpenUsage.ai / CrossUsage 打 `POST api2.cursor.sh/oauth/token`（写死 client_id），写回 sqlite/keychain | **不学写回**，会和 Cursor 抢 refresh |
| Codex | 401 → `codex login` | UsageAtlas 交给 CLI 进程；OpenUsage.ai / ai-usagebar 自己 refresh 写回，已经撞过 `refresh_token_reused` | **学 UsageAtlas：别碰文件** |
| Grok | 标准 OIDC，原子写回同一份 `auth.json` | OpenUsage 也写回但路径写死；ai-usagebar SuperGrok 只读 | 保持现状（这就是官方 grok CLI 的协议） |
| Command Code | 无会话文件 | ai-usagebar **明确只读**，过期报过期 | **学只读** |
| Claude | 无 | ai-usagebar 会 refresh；旋转后的 refresh_token 写失败视为登出 | 若以后做 Claude，refresh 要 fail-closed |

原则：**不要把官方 CLI 的 token 写回官方文件，除非你就是那个 CLI。** leeway 现在只对 Grok（以及 Kimi CLI）写回，路径隔离或原子更新。

---

## 4. 用量：官方窗口 vs 本地账单

别人常把官方窗口和本地账单画在同一张用量图上。leeway 只做官方窗口。

### Cursor

- leeway：Bearer `POST api2.cursor.sh/…/GetCurrentPeriodUsage`。身份 `GetMe`。
- OpenUsage.ai：同一 RPC，再打 `GetPlanInfo` / `GetCreditGrantsBalance` / `GetSandUsageStatus`。Connect 被挡时用 JWT 拼 cookie `WorkosCursorSessionToken={userId}%3A%3A{jwt}` 打 `cursor.com/api/*`。
- UsageAtlas / ai-usagebar：`GET cursor.com/api/usage-summary` + 合成 cookie。

cookie 那条对 raw Bearer 是 401（本机已测）。必须先拆 JWT `sub`。api2 对多账号 CLI 会话更稳。OpenUsage 多出来的是赠额、团队美元池、Grok Bot 周窗口。

### OpenCode Go

- leeway / OpenUsage.ai / ai-usagebar：`GET opencode.ai/zen/go/v1/usage`。
- OpenUsage.sh：**没有**这条。打 `/zen/v1/models` + 控制台月度 USD。
- UsageAtlas / CrossUsage（无 key）：本地花费 ÷ `$12 / $30 / $60`，标成「5-hour (local)」。
- AIUsageTracker：`api.opencode.ai/v1/credits`（按量点数）或 `opencode stats`。

假配额不要学。PAYG credits 和 Go 套餐是两件事，最多当旁路。

### Command Code GOAT

ai-usagebar 是除 leeway 外唯一认真接官方窗口的：

```
GET /alpha/whoami                  # orgId；个人 org=null
GET /alpha/billing/credits         # 主：windowLimits.fiveHour/weekly + monthlyCredits
GET /alpha/billing/subscriptions   # planId → GOAT / Pro
```

- 5h/周用 API 的 `used/cap` 美元，不是写死 14/35。
- 月额度 API **不给上限**，本地表：`individual-goat → $70`。未知 plan 只显示剩余，不编分母。
- **不用** `/alpha/usage/summary` 当窗口（那是累计 totals）。
- token 只读，不写回。

leeway 已有 credits + 可选 summary。缺：读官方 `auth.json`、whoami/plan 名、`$used of $cap`。

### Grok

- leeway：订阅池 `GET cli-chat-proxy.grok.com/v1/billing?format=credits`，4xx/5xx 再 grpc-web。
- OpenUsage.ai：同一 REST，无 grpc-web；缺 `creditUsagePercent` 当 0；非 Weekly 周期不画周窗口。
- OpenUsage.sh：`XAI_API_KEY` → `api.x.ai/v1/api-key` 预付费，**不是** Grok 订阅。
- ai-usagebar 拆成两个 provider：`grok` = Management 预付费，`supergrok` = 订阅。命名更清楚。

### DeepSeek

大家都打 `GET api.deepseek.com/user/balance`。leeway 还多了平台 token 的当日用量，并避开 `platform.deepseek.com` 的 WAF。CrossUsage 要用户填「初始余额」再画 used/limit——假配额。

### Codex

- leeway / 多数：读 `auth.json` → `chatgpt.com/backend-api/wham/usage`。
- UsageAtlas：spawn `codex app-server`，RPC `account/rateLimits/read`，**不打开 OAuth 文件**。
- 窗口按 **时长** 认 5h/7d，不能按 JSON 槽位（OpenAI 曾把周窗口塞进 `primary`）。leeway `windows.go` 已做。

### Claude（leeway 没有）

`GET api.anthropic.com/api/oauth/usage`，头 `anthropic-beta: oauth-2025-04-20`，UA 必须像 `claude-code/…`（缺了会硬 429）。窗口：5h、周、Sonnet 周、额外 credits。

---

## 5. 该学 / 不该学

### 本仓库约束（2026-09-03）

- **有 key 用 key。** 不扫、不复用本机官方工具自己的登录态（`~/.commandcode/auth.json`、`~/.codex/auth.json`、Cursor `state.vscdb` / 默认 keychain、OpenCode `auth.json`、Claude 钥匙串）。
- 登录只走 **leeway 自己隔离的凭据**：粘贴/配置的 API key，或 `leeway auth login` 写进私有 HOME 的文件。
- 多账号靠命名账号 + 私有目录，不 Implicit 读 `default`。

按这个筛过之后，竞品里「读官方 CLI 文件 / spawn 官方 CLI」整条划掉。还值得做的只剩：已经有凭据之后，把官方接口用全、显示对、缓存别串号。

### 该学（按性价比）

1. **Command Code 接官方 CLI 登录**（ai-usagebar）— **划掉（凭据）**；用量口径仍要：  
   用已有 API key 打 whoami（`orgId`）、subscriptions（plan 名）、credits 的 `$used of $cap`；月额度用 plan 表。不读 `~/.commandcode`，不用 summary 当窗口。

2. **Codex 把刷新交给官方 CLI**（UsageAtlas）— **划掉。** spawn `codex app-server` 就是依赖本机官方登录态。

3. **缓存按账号隔离**（ai-usagebar）  
   现在一份 `snapshots.json`、60s。换一把 Go / GOAT key，短时间内可能看到上一号。缓存键加上账号名或 `sha256(key)` 即可。

4. **Cursor 只加读、不加写**（OpenUsage.ai）— 只保留「同一把隔离 token 多打几个官方接口」。  
   同一次 Bearer 可加 `GetPlanInfo` / `GetCreditGrantsBalance` / `GetSandUsageStatus`。不要 IDE sqlite、不要默认 keychain、不要 `oauth/token` 写回。Connect 被挡时用**这把** JWT 拼 cookie 是退路，不是去刮 Cursor 应用。

5. **发现，不当成隐式 default**（OpenUsage.sh）— **划掉自动发现。** 继续：不写 `--account` 就不读 `default`。OpenCode key 只来自 env / `config set-api-key --account`。

6. **Grok 展示别瞎猜**（OpenUsage.ai）  
   没有 `creditUsagePercent` 当 0；周期不是 Weekly 就不画周窗口；`onDemandCap` 可当一行。grpc-web 回退留着。凭据仍是 leeway 隔离的 grok 登录 / 文件，不读用户全局 `~/.grok` 当 default。

7. **以后若做 Claude**（ai-usagebar）— 不能靠本机 `claude` 登录态。要么官方 key 能查出 Code 窗口（目前不能），要么像 Cursor 一样 `leeway auth login` 写进私有 HOME。未做之前不扫钥匙串。

### 不该学

- 自实现 Cursor / Claude / Codex 的 PKCE，或写死 `client_id` 去 refresh
- 把 token 写进 `state.vscdb`、Codex `auth.json`、Claude 钥匙串、Command Code `auth.json`
- 把密钥拷进第二份 `credentials.json` / 加密 vault 当 Cursor/Claude 主路径
- 本地 JSONL / `opencode.db` / `$12/$30/$60` 当官方余量
- DeepSeek「请填初始余额」再画 used/limit
- 自动创建 `cursor-ide` / `claude-code` 这种 implicit default
- OpenUsage.sh 的 daemon、定价表、36 家、tmux
- AIUsageTracker 往 `~/.opencode/auth.json` 回写扫描结果
- 乱刮无关产品的浏览器 cookie

---

## 6. 和 leeway 的位置

产品边界已经选对：**官方余量 CLI，原生窗口，登录委托官方，多账号隔离。** 别人更像本机 AI 账单观察器。

差距不在架构，在三处实现：

1. Command Code 还停留在粘贴 studio key、没读官方 CLI `auth.json`。
2. Codex 还在自己读 OAuth 文件；UsageAtlas 证明可以完全不碰。
3. Cursor 的 Connect RPC 主路是对的；赠额 / 团队池 / 企业 cookie 退路还没接。刷新写回不要做。

OpenUsage / UsageAtlas 适合「看 Claude + 本地烧了多少」。Go / GOAT / DeepSeek / Grok / Cursor 多号，继续以 leeway 为主力。要从他们身上拿的是凭据复用和窗口口径，不是再做一套仪表盘。
