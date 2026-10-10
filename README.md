# leeway（余量·裕度）

[![License: MIT](https://img.shields.io/badge/license-MIT-6e5aff?style=flat-square)](LICENSE)

查 AI 编程额度还剩多少：用量、余额、何时重置。CLI 优先（JSON 优先，stdout 只出数据），也带一个原生窗口界面。

支持：**deepseek / kimi / grok / codex / cursor / opencode-go / commandcode**。

复刻 [CodexBar](https://github.com/steipete/CodexBar) 的探测核心。当前只有窗口进程：关掉就退出，没有菜单栏常驻或后台服务。

## 安装

```sh
go install ./cmd/leeway              # CLI
go build -o leeway ./cmd/leeway
```

GUI 走 [MyGo](https://github.com/egoist/mygo)（纯 Go、无 cgo），要出 `.app` / `.dmg` 得用它的构建器：

```sh
go run github.com/egoist/mygo/cmd/mygo@v0.4.0 build      # build/darwin-arm64/leeway.app + leeway 0.2.0.dmg
go run github.com/egoist/mygo/cmd/mygo@v0.4.0 build -skip-dmg
```

窗口和 CLI 是同一个探测核心、同一份配置：GUI 只多一个 `~/.config/leeway/gui.json`。

## 用法

```sh
leeway help                          # 命令树
leeway help login                    # 某一条的参数
leeway usage                         # 已配置的订阅
leeway usage cursor work
leeway usage --json                  # 给脚本 / agent
leeway usage --last 24               # 近 24 次走势
leeway usage --watch 30

leeway status                        # 谁登了、缺啥（表格给人看）
leeway status --json                 # 给脚本 / agent
leeway login cursor                  # 账号名默认用邮箱；再加 work 是 tag
leeway login grok home
leeway login opencode-go work
leeway login codex                   # 走官方工具的浏览器登录
leeway login kimi
leeway logout cursor work
```

`leeway login cursor/grok/codex/kimi` 调官方工具；其余粘贴 API 密钥。cookie 备选：`leeway login kimi --cookie`、`leeway login grok --manual`。近 N 次走势用 `leeway usage --last N`。

某一条看 `leeway help login`（还有 usage / logout / status / use）。

`leeway status` 的表格是给人看的中文；要机器可读用 `leeway status --json`。JSON 里 `credential` / `status` 的取值仍是英文 token（`ok`、`apiKey (config)`、`authFile (private)` …），只有表格把它们翻成中文，所以别去解析表格。

`PROVIDER` / `ACCOUNT` 也可写成 `--provider` / `--account`。ACCOUNT 是 tag、邮箱（cursor）或订阅名（opencode-go / commandcode）。不写时不读官方 keychain / 共享 `~/.grok`；要看那个号请加 `default`。

`leeway use` 把某个号变成本机正在用的那个，不是切 leeway 自己查哪号：

```sh
leeway use cursor work               # 写本机所有 Cursor 登录位置（见下）
leeway use grok home                 # 回写完整官方 ~/.grok/auth.json（瘦设备码文件会拒绝）
leeway use kimi work                 # 有登录文件则写 ~/.kimi-code；只有密钥则写 DSH
leeway use opencode-go work          # 把密钥写给 DSH
```

macOS / Linux。Cursor 必须先退出（`cursor-agent` 不算，它和 IDE 是两个进程）。`leeway use cursor`
把登录文件写进本机**所有**已有的官方登录位置，缺哪个跳哪个，一个都没有才报错：

- `cursor-agent` 的 keychain 项（macOS `cursor-access-token`）——只用 agent 的机器就靠这个生效
- Cursor IDE 的 `state.vscdb`：macOS `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`，
  Linux `~/.config/Cursor/...`（Flatpak 回退已在 switcher 里）
- `~/.cursor/auth.json`（`AGENT_CLI_CREDENTIAL_STORE=file` 时 agent 读的那份）

`LEEWAY_CURSOR_KEYCHAIN=0` 关掉 keychain 写入（其它值指定 keychain 服务名）；`LEEWAY_CURSOR_STATE_DB` /
`LEEWAY_CURSOR_AUTH_JSON` 各自把对应文件写到别处，指定任一个就不再碰真 keychain；三处都没有时直接失败，不再新建 auth.json。

切换前会先把本机当前登录备份成 leeway 账号，所以切错了能切回来。第一次改动 keychain 项时，
macOS 可能弹一次授权框（那是系统在问要不要让 `security` 写这条记录）。

Grok 官方文件两边都是 `~/.grok/auth.json`。每个号先用官方 grok 登录一次，再 `leeway login grok NAME --file ~/.grok/auth.json` 备份；之后 `use` 只换文件。Codex 只查额度，不写 `~/.codex`。

## 窗口界面

```sh
open build/darwin-arm64/leeway.app        # 或直接跑 ./leeway-gui
```

- **总览**：每个 provider 一张卡，绿/黄/红点 + 进度条 + 重置倒计时；卡片跟着窗口宽度折行
- **provider 页**：账号、登录信息来源、每个窗口的条与花费、缓存/配置/登录文件位置；`登录` / `账号` / `更多` 在这里做。
  一家有多个订阅时换成额度矩阵：列是账号（名字、邮箱、套餐、状态），行是窗口，同一行横向比，一屏看完；
  列间画竖线，窗口名当行头
- **历史**：近 N 次查询的走势图；**关于**：版本与构建信息
- 标题栏：状态汇总、`刷新`、自动刷新间隔、设置；`⌘R` 刷新，`⌘⇧C` 复制当前数据，`⌘,` 打开设置，`⌘W` 关窗
- 设置：自动刷新间隔、查询超时（等价 `LEEWAY_TIMEOUT`）、主题、开机自启、失败通知、每个 provider 的添加/移除开关、历史点数、缓存清理
- 登录/移除账号走和 CLI 一样的三条路：官方工具、粘贴 API 密钥、浏览器登录状态或手动粘贴登录状态/登录文件

GUI 从不显示密钥内容，只说明登录信息来自哪儿（环境变量 / 配置里的 API 密钥 / 浏览器登录状态 / 登录文件）。

## 各家怎么登录

窗口里的按钮和菜单按 provider 声明的登录信息类型长（`AuthSpec`），所以分成两类：

| 登录方式 | Provider | 命令 |
|---|---|---|
| 官方工具登录（窗口按钮写「登录」） | cursor / grok / codex / kimi | `leeway login cursor` / `grok` / `codex` / `kimi` |
| 只用 API 密钥（窗口按钮写「设置密钥」） | deepseek / opencode-go / commandcode | `leeway login …`（粘贴 API 密钥） |
| grok / kimi 备选 | — | `leeway login grok --manual` / `leeway login kimi --cookie` |

## 配置

- 配置：`~/.config/leeway/config.json`（0600，`LEEWAY_CONFIG` 可覆盖）
- 缓存：`~/.cache/leeway/`（`LEEWAY_CACHE`）
- 窗口设置：`~/.config/leeway/gui.json`（0600，CLI 不读也不写）
- 登录信息：**环境变量 → 配置文件**。cookie 只在你显式导入或 `--manual` 时写入缓存。

```json
{
  "apiKeys": { "deepseek": "sk-..." },
  "providers": {
    "opencode-go": {
      "accounts": [
        { "name": "work", "apiKey": "sk-..." },
        { "name": "personal", "apiKey": "sk-..." }
      ]
    }
  }
}
```

## 给脚本用

- stdout 只有表格或 JSON；日志和错误走 stderr
- `leeway usage --json`：`providers[].accounts[].snapshot.windows[]`（`usedPercent` / `used` / `limit` / `resetsAt`，可选 `delta`）
- `leeway usage --last N --json`：近 N 次按窗口的时间序列（`series[].points[]`）；`history` 是这条的别名
- 退出码：`0` 全部正常；`1` 有未配置或失败；`2` 用法错误
- 超时默认 15s（`LEEWAY_TIMEOUT`），无 TTY 自动纯文本，`COLOR=0` 关颜色

## 文档

| | |
|---|---|
| [docs/DESIGN.md](docs/DESIGN.md) | 架构与数据模型 |
| [docs/GUI.md](docs/GUI.md) | 窗口界面：入口、页面、设置、构建 |
| [docs/PROVIDER-FETCH.md](docs/PROVIDER-FETCH.md) | 各家端点与登录信息 |
| [docs/ACKNOWLEDGMENTS.md](docs/ACKNOWLEDGMENTS.md) | 感谢与参考 |
| [docs/](docs/) | 调研与竞品对照 |

## 许可证

[MIT](LICENSE)。Copyright (c) 2026 helixliu。探测核心复刻自 Peter Steinberger 的 CodexBar / SweetCookieKit（同为 MIT），见 [NOTICE](NOTICE)。
