# leeway（余量·裕度）— CodexBar 纯 CLI 复刻（设计文档）

> 复刻 [steipete/CodexBar](https://github.com/steipete/CodexBar) 的核心（AI 编程 provider 额度/余额/重置探测），
> 砍掉 macOS 菜单栏 UI 与周边，做成纯 CLI：简单输出必要信息、JSON 优先，方便集成到 Hermes 等 agent。
> 支持 provider：**deepseek / kimi / grok / codex / cursor / opencode-go / commandcode**（预留扩展位）。
> 认证全做：API key、本地文件、CLI 会话复用、委托官方 CLI 登录（cursor / grok）、浏览器 cookie 导入（L0/L1/L2）。
> 许可证 MIT，上游版权见 [NOTICE](../NOTICE)，感谢与参考见 [ACKNOWLEDGMENTS.md](ACKNOWLEDGMENTS.md)。

## 1. 从 CodexBar 的取舍

| 保留（复刻） | 简化 | 砍掉 |
|---|---|---|
| Provider 探测体系（descriptor + fetch strategy 思想） | 数据模型（只留 RateWindow/Snapshot，砍 ProviderDetailSection 等扩展结构） | 菜单栏 UI、SwiftUI、Widget、Sparkle 更新、iCloud 同步、Hooks、用户插件、图标渲染、不自建 Keychain（0600 文件替代）；cursor 无私有 auth.json 时仍可读官方 keychain |
| 多窗口语义（primary/secondary/tertiary + reset 时间） | 自适应刷新（固定 TTL + watch 间隔） | 通知、confetti、统计分享 |
| 认证适配（env key / cookie / CLI 会话复用） | 呈现层（一张表 + JSON） | 成本历史图表、历史 pace 预测 |
| 多账户机制（OpenCode Go 多 session token） | — | — |

## 2. 技术选型

**Go**：单静态二进制、启动毫秒级、goroutine 并行探测、stdlib 覆盖 HTTP/JSON。

```
依赖: modernc.org/sqlite（纯 Go 无 CGO）；gopkg.in/yaml.v3（`leeway use` 打补丁凭据文件）；golang.org/x/term（TTY）
```

> 备选：Python 开发最快（browser_cookie3 现成）但分发/启动差；Rust 更费工。Go 是平衡点。

## 3. 目录结构

```
leeway/
  README.md LICENSE NOTICE AGENTS.md CLAUDE.md
  docs/                     # 设计、探测实现、调研、感谢
  cmd/leeway/main.go        # 入口，装配 registry
  internal/model/           # RateWindow / Snapshot / JSON 契约
  internal/provider/        # Provider 接口 + Registry
    deepseek/  kimi/  grok/  codex/  cursor/  opencodego/  commandcode/
  internal/auth/            # cookie importer、cursor/grok 私有会话目录
  internal/config/          # ~/.config/leeway/config.json（0600）
  internal/credwrite/       # `leeway use` 打补丁 YAML 凭据
  internal/fetch/           # 并行探测编排
  internal/cli/             # 子命令与 flag
  internal/render/          # json / 表格
```

## 4. 数据模型

```go
// 对应 CodexBar UsageSnapshot：primary/secondary/tertiary 三档窗口 + 可选附加窗口
type Snapshot struct {
    Primary, Secondary, Tertiary *RateWindow
    Extra        []NamedRateWindow
    Cost         *Cost           // 余额/费用（deepseek 余额、opencode zen balance）
    Identity     *Identity       // 账号邮箱等
    UpdatedAt    time.Time
}

// 对应 CodexBar RateWindow
type RateWindow struct {
    ID            string     // "primary"/"secondary"/"tertiary"/自定义
    Title         string     // "Weekly" / "5-hour" / "On-demand"
    UsedPercent   *float64   // 0-100，nil = 非百分比窗口（如余额）
    Used          *float64   // 原始数值（余额 ¥12.34）
    Limit         *float64
    Currency      string     // "CNY"/"USD"
    ResetsAt      *time.Time
    WindowMinutes *int
}
```

### Provider 接口（扩展点）

```go
type Provider interface {
    ID() string                    // "deepseek"
    DisplayName() string
    AuthSpec() AuthSpec            // 声明式：env key、cookie 域名、CLI 会话复用、是否支持多账户
    DeclaredWindows() []WindowSpec // 声明档位，探测返回后按 ID 归位
    Fetch(ctx context.Context, creds Credentials) (*Snapshot, error)
}
```

新 provider 声明 `EnvAPIKey` / `EnvAuthToken`、`CookieDomains`、本地会话等字段后，doctor 和凭据解析按这份声明判断缺什么，不必再写一套解析代码。

## 5. 七个 provider 探测方案

| Provider | 认证来源 | 数据源 | 窗口 | 关键细节 |
|---|---|---|---|---|
| **deepseek** | `DEEPSEEK_API_KEY` env 或 `config set-api-key`（交互式无回显 / --key 直传） | **`api.deepseek.com/user/balance`**（公共 API，Swift 的 API-key 路径；`platform.deepseek.com` 有 WAF 会拦 API key）+ 平台 token 增强：`/api/v0/usage/amount` + `/api/v0/usage/cost` | 余额（¥）+ 当日用量 | 公共 API，余额单位 ¥ |
| **kimi** | 四选一：`KIMI_CODE_API_KEY`（Code API）/ 网页 JWT（cookie `kimi-auth` 或 `KIMI_AUTH_TOKEN`）/ L1 桌面端 / **Kimi CLI 会话**（`~/.kimi-code/credentials/kimi-code.json`，过期提示 `kimi login`） | `www.kimi.com/apiv2/kimi.gateway.billing.v1.BillingService/GetUsages`（周配额，**POST**）+ `kimi.gateway.membership.v2.MembershipService/GetSubscriptionStats`（5h 限速，best-effort） | Weekly 7d + 5-hour + monthly/7d 附加 | JWT 自带 session 信息；两请求并行 |
| **grok** | 三选一：**`leeway auth login --provider grok`**（私有 `GROK_HOME`，OIDC bearer，过期自动刷新到 `{issuer}/oauth2/token` 并回写）/ 官方 `~/.grok/auth.json`（共享）/ L2 Chrome cookie / 手动 header | 优先 `GET cli-chat-proxy.grok.com/v1/billing?format=credits`（`creditUsagePercent` + `currentPeriod.type`）；REST 4xx/5xx 回退 grok.com grpc-web 启发式 protobuf | 共享额度；窗口类型来自 REST `currentPeriod.type`（Weekly/Monthly） | 实测 REST `creditUsagePercent` 不再被 protobuf 截成整数 |
| **cursor** | 委托官方 `cursor-agent login`（`leeway auth login --provider cursor [--account]`：`AGENT_CLI_CREDENTIAL_STORE=file` + 私有 HOME，只读 `.cursor/auth.json`）；env `CURSOR_ACCESS_TOKEN` / `accounts[].apiKey`；**无私有 auth.json 时**才兜底 macOS keychain | `POST api2.cursor.sh/aiserver.v1.DashboardService/GetCurrentPeriodUsage`（Bearer） | Monthly（API bucket %）+ Total/Auto（JSON）+ Credits（`limit − includedSpend`） | 不自实现浏览器 PKCE/poll；60d JWT 过期提示重登 |
| **codex** | 复用 Codex CLI 已登录会话：`~/.codex/auth.json` OAuth token（免自实现 device flow） | `chatgpt.com/backend-api/accounts`（身份）+ `chatgpt.com/backend-api/wham/usage`（窗口） | Session（5h）/ Weekly（7d）+ 附加窗口 + Credits | 复用 Codex CLI 会话，不自实现 OAuth |
| **opencode-go** | **双路径**：① 订阅 API key（`OPENCODE_GO_API_KEY` / 命名订阅 `config set-api-key --account`），走网关 `GET opencode.ai/zen/go/v1/usage` 直接查每订阅用量；② **多账户 cookie**：每个账户 = 一组 session cookie（浏览器 profile 或手动 header）+ 可选 workspaceID（CLI 的 API key 走不通 `_server`，服务器窗口需要真实浏览器会话） | ① 网关 `/usage`（每订阅）② 服务器窗口：`opencode.ai/_server`（Zen balance 等 API，按账户） | 每订阅/每账户：5-hour + Weekly + Monthly；cookie 账户另有 Zen 余额 | 多账户并发 ≤3 + TTL 缓存；workspaceID 缺失要 doctor 提示；`leeway use` 切换激活订阅并写入 DSH 凭据 |
| **commandcode** | `COMMAND_CODE_API_KEY` / `accounts[].apiKey`（`user_...`，commandcode.ai/studio） | `GET api.commandcode.ai/alpha/billing/credits`（`windowLimits.fiveHour/weekly` + `credits.monthlyCredits`）+ 可选 `/alpha/usage/summary` | 5-hour + Weekly + Monthly credits | 纯 API-key 多账号，同 opencode-go 订阅模式 |

浏览器授权（cursor / grok）一律委托官方 CLI 完成，leeway 只读落盘文件。每账号一个私有 HOME / `auth.json`，与官方 CLI / keychain 解耦，切号只换 `--account`。

## 6. Cookie 导入方案（L0/L1/L2 分级）

| 级别 | 方案 | 成本 | 适用 |
|---|---|---|---|
| **L0** | 手动粘贴 Cookie header：`cookie set --provider grok --manual 'xxx=yyy; …'` | 零代码 | 兜底，永远保留 |
| **L1** | 读 provider 自己桌面端存储：Kimi 桌面端 `~/Library/Application Support/kimi-desktop/Cookies`（Chromium 库，直接只读 SQLite 取 `kimi-auth`，**绕开浏览器和钥匙串**） | ~50 行 | kimi |
| **L2** | Chrome/Edge SQLite + macOS 钥匙串解密 | ~150 行，stdlib 可完成 | grok / opencode-go / kimi |

L2 步骤（macOS）：
1. `security find-generic-password -s "Chrome Safe Storage" -w` 取密钥（首次弹授权一次；或用 go-keyring）
2. `modernc.org/sqlite` 只读打开 `~/Library/Application Support/Google/Chrome/<Profile>/Network/Cookies`，按 `host_key LIKE '%域名%'` 过滤
3. 解密 `encrypted_value`：`v10` 前缀 = PBKDF2-SHA1（salt "saltysalt"、1003 迭代、16 字节）+ AES-128-CBC（IV = 16 个空格、PKCS7）；老格式 = 同一 CBC 方案无 v10 前缀。注意 macOS 上 v10 包装的是 CBC 而非 Windows/Linux 的 AES-256-GCM（chrome.go 有注释提醒该坑）；user_version ≥ 24 的库还要剥离 SHA-256(host_key) 前缀
4. 拼 `name=value; …` header 存入 config（0600）

注意：多 profile 扫描（`Default`/`Profile 1`…）；WAL 模式要 immutable/只读打开（Kimi 代码已验证该坑）；cookie 会过期 → `--force` 重导 + fetch 失败报"cookie 过期"。Linux 上 L2 走 libsecret/KWallet，先标记 macOS 优先，Linux 用 L0/L1。

## 7. 多账户机制（opencode-go）

原版验证：`TokenAccountSupport("Store multiple OpenCode Go Cookie headers")`，
`fetchUsage(cookieHeader:, workspaceIDOverride:)` 按账户参数化 → 多账户 = 循环调用。

- **账户 = 一次浏览器登录会话（一组 cookie）+ 可选 workspaceID**
- **订阅 = 一个命名 API key**（`accounts[].apiKey`），走网关 `GET /zen/go/v1/usage` 按订阅直接查 5h/weekly/monthly，无需 cookie
- 服务器窗口按账户各一份
- `cookie import --provider opencode-go` 扫描全部 Chrome profile，每个登录态 = 一个账户候选
- 多账户并发 ≤3；`--account work` 可过滤
- 快速切换：`leeway use --provider opencode-go --account <订阅>` 把 key 写入 DSH 凭据（`keyTarget`，可配置）

cursor / grok 的多账号见 §5：每账号一份私有 HOME / `auth.json`，与官方 CLI / keychain 解耦。

## 8. CLI 设计（纯 CLI，无交互，agent 友好）

```
leeway usage [--provider X] [--account Y] [--json] [--watch 30s] [--no-delta]  # 查额度
leeway history [--last N] [--provider X] [--account Y] [--json]   # 近 N 次点阵
leeway auth login|logout|status --provider X [--account Y]        # 登录 / 清私有会话 / 订阅台账
leeway providers [--json]                             # 各家怎么登录
leeway cookie set|import|list|clear --provider X     # grok/kimi cookie 备选
leeway config path | set-api-key | keys
leeway use --provider X --account NAME                # 把 key 写给 DSH（不是切 leeway 看哪号）
leeway doctor                                         # 缺什么凭据
leeway help COMMAND                                   # usage / auth / cookie / config / use
```

**agent 契约**：stdout 只输出数据、日志走 stderr；退出码 `0` 全部成功 / `1` 部分失败 / `2` 配置错误；无 TTY 自动降级纯文本。

```json
{
  "version": 1,
  "generatedAt": "2026-08-06T10:30:00Z",
  "providers": [{
    "id": "opencode-go", "status": "ok", "error": null, "updatedAt": "…",
    "accounts": [{
      "name": "work", "status": "ok",
      "snapshot": {
        "windows": [{"id": "primary", "title": "5-hour", "usedPercent": 41,
                     "used": null, "limit": null, "currency": null,
                     "resetsAt": "…", "windowMinutes": 300}],
        "cost": {"period": "Zen balance", "currency": "USD", "amount": 18.5}
      }
    }]
  }]
}
```

人看的输出（`--watch 30` 清屏重绘，无按键交互）。`RenderTable` 按账号自己的窗口画：多账号时 provider 名出现一次作组头，tag 各占一行并缩进。

```
OpenCode Go
  github
    Weekly  ⣿⣿⣀  10%  7d
  work
    Weekly  ⣿⣿⣀  10%  7d

DeepSeek
  Balance  ¥12.34
```

## 9. 配置与缓存

- `~/.config/leeway/config.json`（0600）：provider 开关、cookieSource、accounts[]、apiKeys
- 凭据解析优先级：**env var → config 文件**（agent 场景 env 优先）
- 缓存 `~/.cache/leeway/snapshots.json`：TTL 60s；`--watch N` 每 N 秒刷新

## 10. 里程碑

| 阶段 | 内容 | 验收 |
|---|---|---|
| **M0** | 骨架：model、Provider 接口、registry、`usage --json` 空跑 | 合法 JSON schema |
| **M1** | deepseek（env key + set-api-key 管线）+ `doctor` + config | 真实余额可见 |
| **M2** | auth 框架：L0 手动 cookie + L1 kimi 桌面端 + L2 Chrome 导入器 | `cookie import` 一条命令拿到 grok header |
| **M3** | kimi / grok / opencode-go（含多账户 + SQLite 只读） | 三家真实数据 + watch |
| **M4** | codex（复用 CLI 会话） | 多档窗口 + 过期提示 |
| **M5** | 打磨：超时、退出码、agent 集成文档、README | Hermes 侧一次调用可用 |

## 11. 风险与注意

- macOS 钥匙串/TCC：L2 首次弹授权（可接受）；doctor 区分"未配置"与"已配置但失败"
- cookie 过期（grok 会话短）：`import --force` + 明确报错
- WAL 模式 SQLite：cookie 库只读打开 + immutable 回退
- 反爬限流：chatgpt.com / grok.com / opencode.ai 保守默认间隔 + TTL
- 隐私：cookie/token 文件 0600
