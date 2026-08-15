# Provider 额度与凭据获取详解

> 本文聚焦 leeway 最核心的一条链路:**每个订阅 provider 的额度(quota)是怎么拿到的、用什么凭据(Cookie / API key / CLI 会话)去拿、拿不到时怎么报错**。
> 想了解整体架构 / 数据模型 / 里程碑,看 [DESIGN.md](DESIGN.md);本文是它的"实现级"补充,按代码路径走。

## 0. 一次 `leeway usage` 的完整数据流

```
cmd/leeway/main.go
  └─ internal/cli       解析 flags、子命令
      └─ internal/fetch.FetchAll     编排层(唯一入口)
          ├─ 1. 按 AuthSpec 解析凭据  →  ResolveCredentials (env → config → cookie cache)
          ├─ 2. 判断是否够资格探测      →  HasCredentials(不够 = "not_configured")
          ├─ 3. 并行调每个 provider    →  p.Fetch(ctx, FetchRequest{Creds, Account, Extra})
          │    每个 provider 内部:发 HTTP → 解析 → 投影成 model.Snapshot
          └─ 4. 汇总 → model.Output → JSON / 表格
```

关键约定(`fetch.go`):
- 每个 provider 并行探测;同一 provider 的多账户并发 ≤ `MaxAccountConcurrency`(3),防反爬。
- 单次探测超时默认 15s(`LEEWAY_TIMEOUT` 覆盖,上限 3600s)。
- 快照缓存 `~/.cache/leeway/snapshots.json`,TTL 60s;**只有不带 provider/account 过滤的全量跑**才会读写缓存,过滤跑绝不碰缓存(`fetch.go:145`)。

---

## 1. 统一凭据解析层(fetch + auth)

每个 provider 用 `AuthSpec` **声明式**描述自己需要什么凭据(`provider.go:35`),编排层、doctor、cookie 命令都从它推导行为,新 provider 不用新写解析代码。

```go
type AuthSpec struct {
    EnvAPIKey        []string // 例如 ["DEEPSEEK_API_KEY"]
    EnvAuthToken     []string // JWT/会话 token 的 env 名
    UsesCookies      bool     // 用 Cookie header 探测
    CookieDomains    []string // 例如 ["grok.com"]
    CookieFallbacks  []string // 额外回退源,doctor 会提示
    UsesCodexCLI     bool     // 复用 ~/.codex/auth.json
    LocalAuthFiles   []string // 本地 CLI 会话文件(provider 自己解析)
    LoginHint        string   // 会话失效时的修复命令
    SupportsAccounts bool     // 是否多账户
}
```

### 1.1 凭据解析优先级(`fetch.go:64 ResolveCredentials`)

对每类凭据按 **env 变量 → config 文件** 的顺序取值:

| 凭据字段 | 来源 |
|---|---|
| `APIKey` | `EnvAPIKey[0]` env → `config.apiKeys[id]` |
| `AuthToken` | `EnvAuthToken[0]` env → `config.apiKeys[id]` |
| `CookieHeader` | `auth.ResolveCookieHeader`(见 §3.6,不含 config 的 manual header 兜底逻辑在 config 里) |
| `CodexAuth` | 读 `~/.codex/auth.json`(`fetch/codex_auth.go`) |

### 1.2 资格判定(`fetch.go:105 HasCredentials`)

够资格的条件(任一满足即真):有 API key / 有 AuthToken / 有 CookieHeader / 有 Codex CLI token / 存在任一 `LocalAuthFiles` 声明的文件。

**`not_configured` vs `error` 的区分**:编排层在调 `Fetch` 前先看 `HasCredentials`,不够就整包标 `not_configured`,并给出 `missingCredentialHint`(`fetch.go:298`,含"缺啥 → 跑哪条命令");进了 `Fetch` 之后失败才是 `error`。

### 1.3 兜底约定

- 部分探测是 best-effort:主数据失败即整体失败,但附加数据(如 deepseek 当日用量、kimi 订阅统计)失败只降级不失败。
- 反爬与隐私:所有 cookie/token 文件 0600;HTTP 客户端带**同源重定向守卫**(codex / opencode-go),Authorization/Cookie 不会泄漏到外域。

---

## 2. Cookie 获取体系:L0 / L1 / L2

支持 cookie 的 provider 只有三家:`grok` / `kimi` / `opencode-go`(`auth/importers.go:8 DomainsForProvider`)。

| 级别 | 方案 | 适用 | 代码 |
|---|---|---|---|
| **L0** | 手动粘贴 Cookie header(`cookie set --provider X --manual 'a=b; c=d'`) | 永远可用,兜底 | config 的 `ManualCookieHeader` |
| **L1** | 读 provider 自带桌面端存储(Kimi 桌面端 cookie 库) | kimi | `auth/kimi_desktop.go` |
| **L2** | Chrome/Edge SQLite + macOS 钥匙串解密 | grok / kimi / opencode-go | `auth/chrome.go` |

### 2.1 L0 手动 header

存在 config(`providers.<id>.manualCookieHeader`,或账户级 `accounts[].manualCookieHeader`),不落 cookie 缓存。`cookie clear` 只清导入缓存,**手动 header 需手动删 config**。

### 2.2 L1 Kimi 桌面端(`auth/kimi_desktop.go`)

- 路径:`~/Library/Application Support/kimi-desktop/Cookies`。
- Kimi 桌面端是 Chromium 壳,`kimi-auth` cookie 的 `value` 列是**明文**,无需钥匙串解密。
- 查询固定:`name='kimi-auth'` 且 `host_key IN ('www.kimi.com', '.www.kimi.com', '.kimi.com', 'kimi.com')`,`ORDER BY last_access_utc DESC LIMIT 1`。
- 只读打开,`-wal`/`-shm` 缺失时回退 `immutable=1`。

### 2.3 L2 Chrome/Edge(macOS,`auth/chrome.go`)

这是最复杂的部分,完整步骤:

**① 拿钥匙串密钥**
```sh
security find-generic-password -w -s "Chrome Safe Storage"        # Chrome
security find-generic-password -w -s "Microsoft Edge Safe Storage" # Edge
```
首次会弹一次授权;失败时错误信息里直接给 L0 手动 cookie 的替代命令。

**② 定位 cookie 库 + 选 profile**
- 路径:`~/Library/Application Support/Google/Chrome/<Profile>/Network/Cookies`,老版本 `<Profile>/Cookies` 兜底。
- 未指定 `--profile` 时,自动扫 `Default` + `Profile N`,选**匹配目标域名 cookie 最多**的 profile(同数时 Default 赢)。
- SQLite 以 `mode=ro` 打开;若主文件处于 WAL 且两侧车(`-wal`/`-shm`)都没了,回退 `immutable=1`(浏览器干净关闭的遗留坑,`chrome.go:98`)。

**③ 读行 + 去重**
- 按 `host_key LIKE '%域名%'` 过滤(`loadCookieRows`)。
- 去重键 `name|host_key|path`;`preferRecord` 规则:session cookie(expires=0)胜持久 cookie,否则**过期时间更晚**者胜(`chrome.go:248`)。
- 老库没有 `is_httponly` 列,查询失败会去掉该列重试。

**④ 解密 `encrypted_value`**(macOS 特有,CBC 而非 Windows/Linux 的 GCM):

```
key = PBKDF2-HMAC-SHA1(钥匙串密码, salt="saltysalt", 1003 迭代, 16 字节)
iv  = 16 × 0x20(空格)
解密 = AES-128-CBC(key, iv, PKCS7)
```

两个坑都在 `chrome.go:169 decryptValue` 里:
- 现代 blob 带 `v10` 前缀,先剥掉。
- cookie 库 `meta` 表 `version >= 24` 时,明文前 32 字节是 `SHA-256(host_key)`,需要校验并剥离(别用 `PRAGMA user_version`,现代 Chrome 上是 0)。

**⑤ 组装 header**
- 排序确定性:先 session cookie、再 httpOnly,然后按 name(`buildHeader`)。
- 名字/值含控制字符(`< 0x20` 或 `0x7f`)的 cookie 直接丢弃——Go 的 HTTP client 会拒绝整个 header,一个坏 cookie 不能毒化整批导入。
- `decryptRows` 统计 `dropped` 数,用于区分 **"没有匹配 cookie"** vs **"有 cookie 但钥匙串 key 不对/解密全失败"**(后者错误信息会引导重查钥匙串)。

### 2.4 cookie 缓存(`auth/auth.go`)

- 文件:`~/.cache/leeway/cookies.json`(0600)。
- 缓存键:`"providerID"` 或 `"providerID@Profile"`(`cacheKey`),让一个 provider 可以共存多个浏览器会话。
- 新鲜度:12h(`ImportTTL`);`cookie import --force` 无视 TTL 强刷。
- 缓存损坏 ≠ 未导入:`cacheUnreadableError` 会明确说"删掉文件重导",避免用户以为要重新登录。

### 2.5 header 解析优先级(`auth.go:169 ResolveCookieHeader`)

对某个账户最终用哪个 header:

```
账户手动 header (accounts[].manualCookieHeader)
  → 账户绑定的 profile 导入缓存 (accounts[].Profile)
  → provider 级手动 header (providers.<id>.manualCookieHeader)
  → provider 级导入缓存
  → 报错 "run: leeway cookie import / cookie set --manual"
```

另外 `ExtractCookieHeader`(`auth.go:269`)能从用户从 DevTools 粘来的 **curl 片段**(`-H 'Cookie: ...'`、`--cookie`、`-b`)里抽出干净 header,kimi / opencode-go 共用同一套正则,修引号一次到位。

---

## 3. 各 Provider 额度获取实现

### 3.1 deepseek —— API key + 可选平台 token(余额 + 当日用量)

| 项 | 值 |
|---|---|
| 认证 | `DEEPSEEK_API_KEY` / `DEEPSEEK_KEY`(env)或 `config set-api-key`;**可选**平台 token `DEEPSEEK_PLATFORM_TOKEN` / `DEEPSEEK_USER_TOKEN` |
| 主端点 | `GET https://api.deepseek.com/user/balance`(`Authorization: Bearer <apiKey>`) |
| 增强端点 | `GET platform.deepseek.com/api/v0/usage/amount` + `/usage/cost`,`?month=&year=`(UTC),两者**并发且都必需** |
| 窗口 | primary `Balance`(原始金额 + 币种)+ 有 token 时每个 `day-YYYY-MM-DD` 窗口(当日 token 数) |
| 实现 | `provider/deepseek/{stub,fetcher,parser}.go` |

要点:
- **余额必须走公开 API 域** `api.deepseek.com`;`platform.deepseek.com` 有 WAF,会拦 API-key 请求(`fetcher.go` 注释)。
- `parseBalance`(`parser.go:102`)兼容两种响应形状:① `{is_available, balance_infos}`;② 平台 envelope `{code, data:{biz_code, biz_data:{normal_wallets, bonus_wallets}}}`。金额字段可能是**数字或字符串**,有 flex 解析。
- 币种选择规则:优先"有资金的 USD",其次任意有资金币种,再兜底 USD 行(`parser.go:150`)。
- 平台 token 增强失败**只降级为余额快照**,不整体失败(`stub.go:120`)。
- 错误分类:余额 401 → `InvalidAPIKey`;平台 401/403 → `InvalidPlatformToken`;平台业务码 `40002/40003` 也视为认证失败(`parser.go:14`)。

### 3.2 kimi —— 四路认证 + Web/Code API

| 项 | 值 |
|---|---|
| 认证(按优先级) | ① API key(`KIMI_CODE_API_KEY`/config)→ Code API;② **CLI 会话**(`~/.kimi-code/credentials/kimi-code.json`,自动刷新)→ Code API;③ Web JWT(`kimi-auth` cookie 或 `KIMI_AUTH_TOKEN`)→ Web API;④ CLI 会话存在但过期 → 提示 `kimi login` |
| Web 端点 | `POST www.kimi.com/apiv2/kimi.gateway.billing.v1.BillingService/GetUsages`(`{"scope":["FEATURE_CODING"]}`);并发 best-effort `POST .../MembershipService/GetSubscriptionStats` |
| Code API | `GET api.kimi.com/coding/v1/usages`(`Bearer <apiKey>`,base 可被 `KIMI_CODE_BASE_URL` 覆盖) |
| 窗口 | primary `Weekly`(7d)+ secondary `5-hour`(限速,API 窗口时长优先)+ 附加 `Monthly`(订阅池 FEATURE_OMNI)+ `Code 7-day` |
| 实现 | `provider/kimi/{kimi,parse,localsession,token}.go` |

要点:
- Web 请求头里 `Cookie: kimi-auth=<token>`,且从 JWT payload 解出 `device_id` / `ssid` / `sub` 填 `x-msh-device-id` / `x-msh-session-id` / `x-traffic-id`(`token.go decodeSessionInfo`),还带 `connect-protocol-version: 1`、`r-timezone`(`kimi.go:380`)。
- 订阅统计是 **best-effort**:非 200 / 解析失败都返回 `(nil,nil)`,只日志;主 fetch 完成后有 2s grace 等它(`kimi.go:153`)。
- `parseCounts`(`parse.go:151`):`used` 优先、`remaining` 兜底;两个计数都坏时给 `reliable=false` 的 0% 假条。limit 无效则整个窗口不输出。
- 字段全部走 string 强转(`stringValue`),因为 Kimi API 数字有时发字符串。
- 错误:Web 401/403 → `InvalidToken`;Code API 401 → `InvalidAPIKey`。

### 3.3 grok —— grpc-web + 启发式 protobuf 扫描(最"黑盒"的一家)

| 项 | 值 |
|---|---|
| 认证 | ① Cookie header(L0/L2);② 本地 `grok login` 会话 `~/.grok/auth.json`(OIDC bearer,过期自动刷新) |
| 端点 | `POST https://grok.com/grok_api_v2.GrokBuildBilling/GetGrokCreditsConfig` |
| 请求 | body 是 **5 个零字节**;grpc-web 头:`x-grpc-web: 1`、`Content-Type: application/grpc-web+proto`、`x-user-agent: connect-es/2.1.1`;瞬时失败重试 1 次 |
| 窗口 | 单个 primary `Credits`(标题按重置距离标 `Weekly`/`Monthly`) |
| 实现 | `provider/grok/{stub,fetch,grpcweb,protobuf,localsession}.go` |

要点(都在 `protobuf.go`):
- grok 的 billing 响应**没有公开 schema**,靠启发式扫描 wire format:
  - **usedPercent**:遍历 fixed32 字段,取"字段路径以 1 结尾 + 值 ∈ [0,100] + 非 NaN/Inf";路径深度最浅者胜,深度相同取最早出现(`protobuf.go:69`)。
  - **resetsAt**:varint 值落在 `[1.7e9, 2.1e9]`(unix 秒窗口)且在现在之后;优先路径 `[1,5,1]`,否则取最早(`protobuf.go:86`)。
  - **noUsageYet**:完全没有 fixed32,但存在 reset 且带 usage-period 标记(路径 `[1,6,...]` 或 `[1,8,1]` 值为 1/2)→ 判定为 0% 已用。
- 深度上限 4;畸形字段跳 1 字节重同步。
- grpc 层:`grpc-status` 头/trailer 非零 → RPC 错误(`grpcweb.go:60`);值做 percent-decode(`+` 不当空格)。
- 会话刷新(`localsession.go`):OIDC `refresh_token` grant 打到 `{issuer}/oauth2/token`,成功后**格式保真地原子写回** `~/.grok/auth.json`(0600,temp+rename),grok CLI 和 leeway 共用一份会话。
- 标题映射(`stub.go primaryLabel`):重置距离 4–12 天 → `Weekly`,20–45 天 → `Monthly`,其余 → `Credits`。

### 3.4 codex —— 复用 Codex CLI 会话,零自研 OAuth

| 项 | 值 |
|---|---|
| 认证 | `~/.codex/auth.json`(`tokens.access_token`,兼容旧 `OPENAI_API_KEY`;`account_id`),`CODEX_HOME` 覆盖目录 |
| 身份端点 | `GET https://chatgpt.com/backend-api/accounts`(20s 超时) |
| 用量端点 | `GET https://chatgpt.com/backend-api/wham/usage`(30s 超时);base 里不含 `/backend-api` 则走 `/api/codex/usage` |
| base 覆盖 | `chatgpt_base_url` in `~/.codex/config.toml`(简单 `key = value` 解析) |
| 窗口 | primary `Session`(5h)+ secondary `Weekly`(7d)+ 附加 `additional_rate_limits` + `Credits` 余额 |
| 实现 | `provider/codex/{stub,fetcher,parser}.go` |

要点:
- 请求带 `Authorization: Bearer` + `ChatGPT-Account-Id`;`newHTTPClient` 的 **redirect guard 只放行同源 HTTPS 跳转**,防 Authorization 泄漏(`fetcher.go:48`)。
- `parseUsageResponse`(**lossy**,`parser.go:63`):顶层严格,嵌套值坏掉只丢自己、不连累兄弟。`used_percent/reset_at/limit_window_seconds` 三者必须都有才认一个窗口。
- `flexFloat`/`flexInt`(`parser.go:199`)容忍 API 把数字发成 Int/String。
- 兜底:没有 primary/secondary 窗口时,只有 credits 存在快照才存活(`stub.go:75`)。
- 401/403 → `kindUnauthorized`,提示重跑 `codex login`。

### 3.5 opencode-go —— 订阅 API key + 多账户 cookie + 本地成本

| 项 | 值 |
|---|---|
| 认证 | **双路径**:① 订阅 API key(`OPENCODE_GO_API_KEY` env / `config set-api-key [--account NAME]`,多订阅 = 多命名 key);② 每账户 cookie(`auth`/`__Host-auth`)+ 可选 `workspaceID`;`SupportsAccounts` |
| 订阅端点 | `GET https://opencode.ai/zen/go/v1/usage`(OpenAI 兼容网关,`Authorization: Bearer <key>`),返回 `{usage:{rolling,weekly,monthly}}`,每订阅独立,无需 cookie |
| 服务器 | `https://opencode.ai/_server` RPC(带 `X-Server-Id` / `X-Server-Instance: server-fn:<uuid>`),含 workspace 枚举与 Zen billing |
| 页面 | `GET /workspace/<ws>/go`(用量页,解析 HTML 里的用量字段) |
| 本地成本 | `~/.local/share/opencode/opencode.db`(SQLite `mode=ro&immutable=1`,机器级一份,不区分账户) |
| 窗口 | 订阅 key:`5-hour`(rolling)/ `Weekly` / `Monthly`;cookie 账户:同上 + `Renews` + Cost `Zen balance`;provider 级 `localCost` |
| 实现 | `provider/opencodego/{provider,fetcher,local,cookie,zen,parser}.go` |

要点:
- **订阅 key 路径**(`provider.go` `fetchByAPIKey`):`Fetch` 里 `req.Creds.APIKey` 非空即走网关
  `/usage`,`parseAPIUsage`(`parser.go`)按已知形状解析三档窗口,每档独立(缺档/坏档只丢自己,
  rolling 必需);此面没有 Zen 余额。每个 `accounts[].apiKey` 是一个订阅;`leeway use` 把选中订阅
  的 key 写入 `keyTarget`(默认 `~/.dsh/.credentials.yaml` 的 `<PROVIDER>_API_KEY`),DSH 热重载即生效。
  编排层 `overlayAccountCreds` 只把命名 `apiKey` 下发给该账户;cookie / workspace 账户清空继承的
  env/`config.apiKeys` key,避免 `leeway use` 之后混用账户都打到同一订阅。
- 单账户 cookie `Fetch`(`provider.go:76`):
  1. **解析 workspaceID**:配置的优先,否则 GET `_server?id=<workspacesServerID>`,再不行 POST(args `[]`)兜底,取第一个 ID。
  2. 用法页与 **Zen 余额并发**(余额带 25ms 起始延迟,镜像 Swift);用法页缺字段时**余额单独成为快照**(`requiredZenFallback`)。
  3. 可选加入余额:最多等 `optionalZenBalanceTimeout`(5s)。
- 反爬:多账户并发 ≤3 + 快照 TTL;`looksSignedOut` 检测登录失效 → `invalidCredentials`。
- 本地成本只取**最新日历天桶的总额**;库缺失/空 → `(nil,nil)` 静默跳过(`local.go`)。

---

## 4. 本地 CLI 会话复用(非 cookie 的第三种凭据)

三家复用各自的 CLI 登录态,且**都会自动刷新并写回**,与 CLI 共享一份会话:

| provider | 文件 | 刷新机制 | 写回 |
|---|---|---|---|
| grok | `~/.grok/auth.json`(`GROK_HOME` 覆盖) | OIDC `refresh_token` → `{issuer}/oauth2/token` | 原子 temp+rename,0600(`grok/localsession.go`) |
| kimi | `~/.kimi-code/credentials/kimi-code.json`(`KIMI_CODE_HOME` 覆盖) | OAuth refresh → `auth.kimi.com/api/oauth/token`;`client_id` 从 refresh_token JWT 里解 | 原子写回,保留未知字段(`kimi/localsession.go`) |
| codex | `~/.codex/auth.json`(`CODEX_HOME` 覆盖) | **不刷新**,过期提示重跑 `codex login` | — |

两个注意点:
- grok/kimi 的刷新请求都带 `ctx`,受编排层 15s 超时约束,避免挂死的 OIDC 服务器阻塞整个探测。
- kimi 用**哨兵错误**(`errSessionExpired`)+ `errors.Is` 分类,不是字符串匹配——"存在但过期" 与 "根本没登录" 报不同的提示。

---

## 5. 多账户 / 多订阅机制(opencode-go 专有)

- **账户 = 一次浏览器登录会话(一组 cookie)+ 可选 workspaceID**(`config.go AccountConfig`:name / manualCookieHeader / workspaceID / profile)。
- **订阅 = 一个命名 API key**(`accounts[].apiKey`),走网关 `/usage` 端点;与 cookie 账户共存于同一 `accounts[]`。
- 每个账户的 cookie 来源:账户级 `manualCookieHeader` → 账户绑定的 Chrome `profile` 导入缓存(`opencode-go@Profile 1`)→ provider 级(`fetch.go:286 resolveAccountCookie`)。
- `cookie import --provider opencode-go --account work` 会把导入的会话直接绑成一个账户。
- 本地 opencode.db 成本历史是**机器级**的,一份放 provider 级 `localCost`;服务器窗口按账户各一份。
- `--account NAME` 过滤;没配置的账户名直接报 `error`。
- **快速切换**:`leeway use --provider opencode-go --account <订阅名>` 把该订阅 key 写入 `keyTarget`
  (默认 DSH `~/.dsh/.credentials.yaml` 的 `OPENCODE_GO_API_KEY`);`leeway config keys` 列出命名订阅(脱敏,对照目标文件标 active)。
  第一个命名 key 在已有 cookie 会话(手动 header 或 import 缓存)时保留 cookie `default`,避免隐式会话被顶掉。

---

## 6. 失败模式速查(用户看到什么 / 代码在哪)

| 现象 | 原因 | 定位 |
|---|---|---|
| `not_configured` + "set DEEPSEEK_API_KEY..." | 完全没凭据 | `fetch.go:298` |
| "missing Codex session — run: codex login" | `~/.codex/auth.json` 缺失/无 token | `fetch/codex_auth.go` |
| "missing cookies — run: leeway cookie import" | cookie provider 无 header | `auth.go:192` |
| "found N cookies but none could be decrypted" | 钥匙串 key 过期或授权被拒 | `chrome.go:544` |
| "cookie host-key hash mismatch" | 库 version ≥ 24,明文 hash 校验失败 | `chrome.go:187` |
| "grok session refresh failed (HTTP xxx)" | OIDC 刷新失败 | `grok/localsession.go:141` |
| "kimi CLI session expired — run: kimi login" | kimi-code.json 存在但刷新失败 | `kimi/localsession.go:160` |
| `kindUnauthorized`(codex) | wham/usage 返回 401/403 | `codex/fetcher.go:82` |
| "Missing workspace id."(opencode-go) | cookie 有效但服务器拿不到 workspace | `opencodego/fetcher.go:97` |

---

## 附:目录速查

```
internal/auth/             cookie 缓存 + L0/L1/L2 导入器(chrome / kimi_desktop)
internal/fetch/            编排:凭据解析、并行探测、快照缓存;codex_auth 读 auth.json
internal/config/           0600 config.json(manual header、accounts、apiKeys)
internal/provider/         AuthSpec + 注册表
  deepseek/                balance(必)+ 当日用量(选)
  kimi/                    Web/Code API + JWT session 头 + CLI 会话刷新
  grok/                    grpc-web + protobuf 启发式扫描 + OIDC 刷新
  codex/                   wham/usage + accounts(复用 CLI 会话)
  opencodego/              _server RPC + 用量页 + 本地 SQLite + 多账户
```
