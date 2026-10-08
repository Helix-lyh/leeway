# 感谢与参考

leeway 能做成，是因为先有人把「各家额度怎么查」走通了。下面两份名单分开写：感谢是对人/项目的致谢，参考是我们实际对照过的材料。

## 感谢

| 项目 | 作者 | 我们欠什么 |
|---|---|---|
| [CodexBar](https://github.com/steipete/CodexBar) | Peter Steinberger ([steipete](https://github.com/steipete)) | 探测核心、窗口模型和各家策略。leeway 是它的 CLI 优先复刻（另有一个原生窗口），源码注释里的 `mirrors` / `ported from` 都指向这里。 |
| [SweetCookieKit](https://github.com/steipete/SweetCookieKit) | Peter Steinberger | macOS 上 Chrome / Edge cookie 解密（`internal/auth/chrome.go`）。 |
| [ai-usagebar](https://github.com/akitaonrails/ai-usagebar) | Fabio Akita | Command Code / OpenCode Go 的官方窗口口径，以及「有 key 用 key、OAuth 只读不写回」。 |
| [UsageAtlas](https://github.com/SRX9/UsageAtlas) | SRX9 | Codex 刷新交给官方 CLI、不改 `auth.json`。 |
| [OpenUsage.ai](https://github.com/robinebers/openusage) | Robin Ebers | Cursor Connect RPC 主路、Grok REST 订阅池。我们不学它在探测里写回官方 sqlite / keychain。 |
| [ccusage](https://github.com/ryoppippi/ccusage) | ryoppippi | 把「还剩多少」和「已经花了多少」分开：它做本地账单，我们做官方窗口。 |

法律文本见 [NOTICE](../NOTICE) 和 [LICENSE](../LICENSE)。

## 引用与参考

调研时对照过这些仓库或站点。对照不等于移植；移植只发生在 NOTICE 里点名的两份。

### 探测实现对照（官方余量）

| 项目 | 链接 | 对照了什么 |
|---|---|---|
| CodexBar | https://github.com/steipete/CodexBar | 各家 fetcher / parser / 窗口归一 |
| SweetCookieKit | https://github.com/steipete/SweetCookieKit | Chromium cookie 解密 |
| OpenUsage.ai | https://github.com/robinebers/openusage · https://www.openusage.ai/ | Cursor api2 RPC、Grok REST |
| Cockpit Tools | https://github.com/jlcodes99/cockpit-tools | 桌面切号落点：Cursor `state.vscdb` 的 `cursorAuth/*`、Grok 官方 `auth.json` registry 合并。只对照公开路径，不移植其代码 |
| CrossUsage | https://github.com/barramee27/crossusage · https://crossusage.dev/ | 旧 OpenUsage 插件树、多账号库 |
| OpenUsage.sh | https://github.com/janekbaraniewski/openusage · https://openusage.sh/ | 终端仪表盘覆盖面（本地花费档） |
| UsageAtlas | https://github.com/SRX9/UsageAtlas · https://usageatlas.com/ | Codex app-server、不碰 OAuth 文件 |
| ai-usagebar | https://github.com/akitaonrails/ai-usagebar | Command Code credits、OpenCode Go `/usage`、Grok SuperGrok |
| AIUsageTracker | https://github.com/rygel/AIUsageTracker | Windows 托盘 + `act` CLI |
| AIUsage | https://github.com/juliantanx/aiusage | 本地 JSONL / SQLite 账单（对照「不该学」） |

### 本地花费 / 互补工具

| 项目 | 链接 | 对照了什么 |
|---|---|---|
| ccusage | https://github.com/ryoppippi/ccusage | 本地 JSONL 成本分析，和 leeway 互补 |
| Token Monitor / iFence | （调研时本机对照，无稳定上游链接） | 本地日志聚合 |

### 社区提及（未移植）

| 项目 | 备注 |
|---|---|
| AgentSubscriptionUsage | 统一多 provider 查询；Cursor 走 `auth.json` → api2 |
| cclimits | statusline one-liner |
| tokscale | Grok / Kimi / Codex 多账号 TUI |
| pi-quota-monitoring / pi-commandcode-provider | Command Code + OpenCode Go 纯 API key 多账号 |

同类对照的展开见 [COMPETITORS.md](COMPETITORS.md)，各家端点见 [PROVIDER-FETCH.md](PROVIDER-FETCH.md)。
