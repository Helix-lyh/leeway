# leeway（余量·裕度）

AI 编程 provider 用量 / 额度 / 重置时间的纯 CLI 探测工具——[CodexBar](https://github.com/steipete/CodexBar) 核心的 CLI 复刻。
JSON 优先、stdout 只输出数据，方便 agent（Hermes 等）直接调用。

支持 provider：**deepseek / kimi / grok / codex / opencode-go**。

## 安装

```sh
# 方式一：go install（安装到 GOBIN，建议配置 PATH）
go install ./cmd/leeway

# 方式二：本地构建
go build -o leeway ./cmd/leeway && ./leeway usage
```

## 命令

| 命令 | 说明 |
|---|---|
| `leeway usage [--provider ID] [--account NAME] [--json] [--watch SECS] [--no-cache]` | 主命令：探测全部（或指定）provider 的用量 |
| `leeway providers [--json]` | 列出 provider 及其认证方式 |
| `leeway cookie set --provider ID --manual "a=b; c=d"` | L0：手动粘贴 Cookie header |
| `leeway cookie import --provider ID [--browser chrome\|edge\|kimi-desktop] [--profile NAME] [--account NAME] [--force]` | L2/L1：从浏览器或桌面端自动导入 cookie；`--profile` 指定 Chrome profile，`--account` 同时把会话绑定为多账户配置中的一个账户 |
| `leeway cookie list` / `leeway cookie clear --provider ID` | 查看 / 清除导入的 cookie 缓存 |
| `leeway config path` | 显示配置文件路径 |
| `leeway config set-api-key --provider ID [--key VALUE]` | 写入 API key（不传 --key 时交互式无回显输入） |
| `leeway doctor [--json]` | 诊断各 provider 缺什么凭据 |

## 配置

- 配置文件：`~/.config/leeway/config.json`（0600 权限，自动创建）
- 环境变量覆盖：`LEEWAY_CONFIG=/path/to/config.json`
- 缓存目录：`~/.cache/leeway/`（可用 `LEEWAY_CACHE` 覆盖），快照 TTL 60s
- 凭据优先级：**环境变量 → 配置文件**（agent 场景优先用 env）

```json
{
  "providers": {
    "grok": { "cookieSource": "manual", "manualCookieHeader": "sso=...; session=..." },
    "opencode-go": { "accounts": [{ "name": "work", "workspaceID": "ws_..." }] }
  },
  "apiKeys": { "deepseek": "sk-..." }
}
```

## Cookie 导入指南

| 级别 | 方案 | 适用 |
|---|---|---|
| **L0** | `leeway cookie set --provider grok --manual 'xxx=yyy; …'`（浏览器 DevTools 复制 header） | 兜底，永远可用 |
| **L1** | `leeway cookie import --provider kimi --browser kimi-desktop`（直接读 Kimi 桌面端自己的 cookie 库，绕开浏览器和钥匙串） | kimi |
| **L2** | `leeway cookie import --provider grok`（Chrome/Edge SQLite + macOS 钥匙串解密，自动扫全部 profile，cookie 最多的 profile 胜出） | grok / opencode-go / kimi |

注意：cookie 会过期（grok 会话尤其短）——过期后重新 `import --force` 或 fetch 会明确报错；
Linux 上 L2 未实现，用 L0/L1。`cookie clear` 只清导入缓存，手动 header 存于 config，需手动删除。

## Provider 一览

| Provider | 认证 | 数据源 | 窗口 |
|---|---|---|---|
| **deepseek** | `DEEPSEEK_API_KEY`（或 `DEEPSEEK_KEY`、`config set-api-key`）；可选 `DEEPSEEK_PLATFORM_TOKEN`/`DEEPSEEK_USER_TOKEN` 补充当日用量 | `platform.deepseek.com/api/v0` | 余额（¥）+ 当日用量 |
| **kimi** | 四选一：`KIMI_CODE_API_KEY`（Code API）/ 网页会话 JWT（`kimi-auth` cookie 或 `KIMI_AUTH_TOKEN`）/ L1 桌面端 / **Kimi CLI 会话**（`~/.kimi-code/credentials/kimi-code.json`，过期时提示 `kimi login`） | `www.kimi.com/apiv2` gRPC-style JSON | Weekly 7d + 5-hour 限速 |
| **grok** | 三选一：**`grok login` 会话**（`~/.grok/auth.json`，过期自动 OIDC 刷新）/ cookie（L0 手动或 L2 Chrome 导入） | `grok.com` GetGrokCreditsConfig（grpc-web） | Credits 窗口（按重置距离标 Weekly/Monthly） |
| **codex** | 复用 Codex CLI 会话 `~/.codex/auth.json`（无需自实现 OAuth；过期提示重跑 `codex login`） | `chatgpt.com/backend-api`（可用 `chatgpt_base_url` 覆盖，见 `$CODEX_HOME/config.toml`） | Session 5h + Weekly 7d + 额外窗口 + Credits 余额 |
| **opencode-go** | 多账户：每账户一组 cookie（`auth`/`__Host-auth`）+ 可选 `workspaceID` | 服务器：`opencode.ai` 用量页 + Zen 余额；机器级：本地 `opencode.db` 成本历史 | 每账户 5-hour / Weekly / Monthly + Zen 余额；provider 级本地成本 |

## JSON 契约（`leeway usage --json` 真实输出）

无凭据时（全新配置）每个 provider 为 `not_configured`，退出码 1：

```json
{
  "version": 1,
  "generatedAt": "2026-08-06T20:17:05.047677+08:00",
  "providers": [
    {
      "id": "codex",
      "displayName": "Codex",
      "status": "not_configured",
      "error": "missing Codex session — run: codex login",
      "updatedAt": "2026-08-06T20:17:05.047703+08:00",
      "accounts": [
        { "name": "default", "status": "not_configured",
          "error": "missing Codex session — run: codex login" }
      ]
    },
    {
      "id": "deepseek",
      "displayName": "DeepSeek",
      "status": "not_configured",
      "error": "missing credentials — set DEEPSEEK_API_KEY or: leeway config set-api-key",
      "updatedAt": "2026-08-06T20:17:05.047689+08:00",
      "accounts": [
        { "name": "default", "status": "not_configured",
          "error": "missing credentials — set DEEPSEEK_API_KEY or: leeway config set-api-key" }
      ]
    }
  ]
}
```

配置好凭据后，`status` 变为 `ok`，`accounts[].snapshot.windows[]` 携带各档窗口
（`usedPercent`/`used`/`limit`/`resetsAt`/`windowMinutes`），`cost` 携带余额类数据；
opencode-go 另有 provider 级 `localCost`（机器级本地历史）。

## 退出码

| 退出码 | 含义 |
|---|---|
| `0` | 全部 provider ok |
| `1` | 至少一个 provider 报错或未配置（`error` / `not_configured`） |
| `2` | 用法 / 配置错误（未知 provider、未知命令、flag 解析失败） |

## Agent 集成约定

- **stdout 只输出数据**（JSON 或表格），日志与错误提示一律走 stderr
- 探测超时默认 15s（`LEEWAY_TIMEOUT` 秒数可覆盖），多 provider 并行探测
- 无 TTY 自动降级纯文本；`COLOR=0` 可强制关闭颜色

## 设计文档

架构、数据模型、探测方案与里程碑见 [DESIGN.md](DESIGN.md)。
