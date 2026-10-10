# leeway UI 换栈调研：mygo → Flutter 与 Android/iOS 出端（2026-10-09）

> 问题：把 UI 层从 MyGo 换成 Flutter，推出 Android / macOS 端，可行吗？成本多少？
>
> **证据分级**（沿用 [RESEARCH.md](RESEARCH.md) 的约定）：
> `实测`＝在本仓代码上跑过并留下产物；`查证`＝外部资料/源码，附出处；
> `推断`＝无直接验证，已显式标注。
>
> 全程未访问真实 provider HTTP，未读本机 cookie / keychain / 凭据内容。
> 完整 PoC、截图与复现脚本在调研分支 **`analysis/mygo-to-flutter`**
> （工作树 `.worktrees/ui-flutter-lab`），见 §9 / §10。

---

## 0. TL;DR

1. **技术可行，PoC 已在 macOS 与 Android 真跑通**：Go 探测核心编成 `.so`，Flutter
   （纯原生控件，零 mygo 依赖）经 `dart:ffi` 调用，真 APK 装进 Android 16 模拟器跑起来了。
2. **挡住 Android 的从来不是那 13.9k 行核心，是 UI 层的 mygo/purego**：核心在
   `CGO_ENABLED=0` 下直接编到 `android/arm64`；而 mygo + purego 只被 `cmd/leeway-gui` 拉进来。
3. **macOS 早就有了，桌面还白送 Linux + Windows**：`mygo build -platform linux/amd64,windows/amd64`
   零代码改动就产出 `.deb` / `.tar.gz` / `.exe`。**唯一真正新增的平台只有 Android。**
4. **Android 上产品价值塌陷**（实测）：自动发现 **0/7**（macOS 同一份代码是 4/7），
   且 `config.Load()` 因 `os.UserHomeDir()` 返回 `/sdcard` 而直接 `permission denied`。
   这是 OS 沙箱事实，**换语言不解决**。
5. **可行的降级形态是「只查有 API key 的家」**：4 家可粘贴 key 直接用，1 家需桌面端导 token，
   2 家移动端不可用。这条路把移动端要碰的核心代码压到约 5.1k 行（§8）。

**建议**：桌面（`internal/gui` + mygo）一行不动，mobile 另做一个窄的 Flutter 客户端，
且**先改移动端的产品定位**——它是「跨端额度看板」，不是「leeway 手机版」。

---

## 1. 实测证据（`实测`，均在 v0.3.4 依赖树上复验）

| # | 实验 | 命令 | 结果 |
|---|---|---|---|
| E1 | 探测核心交叉编译到 Android | `CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build ./cmd/leeway` | ✅ exit 0，17M aarch64 ELF |
| E2 | mygo 窗口编到 Android | 同上，`./cmd/leeway-gui` | ❌ `purego/dlfcn_android.go:21,25,29: undefined: cgo.Dlopen/Dlsym/Dlclose` |
| E3 | android 实际链进哪个 mygo 后端 | `go list -deps ./cmd/leeway-gui` | `mygo/internal/linux` + `internal/gpu/gl`（GTK 桌面后端），**无** `internal/unsupported` |
| E4 | `c-shared` 是否强制 cgo | `go build -buildmode=c-shared`，CGO=0 / 1 | CGO=0 ❌ `build constraints exclude all Go files`；CGO=1 ✅ |
| E5 | 真实核心包成 c-shared | `lab/bridge` | ✅ 8.6M dylib，从 C 与 Python ctypes 调通，返回真实 JSON |
| E6 | Android `.so`（NDK 30） | `lab/build-android-libs.sh` | ✅ 3 个 ABI：8.3 / 8.4 / 8.7M，`0x4000` 页对齐（16KB） |
| E7 | 真 APK | `flutter build apk --release` | ✅ 74.3MB，`lib/<abi>/libleway.so` 就位 |
| E8 | macOS Flutter app | `flutter build macos --release` | ✅ 41.2MB，FFI 通，**4/7 自动发现** |
| E9 | Android 模拟器真跑 | `adb install` + `screencap` | ✅ FFI 通，**0/7 自动发现**，见 §2 |
| E10 | mygo 出桌面三端 | `mygo build -platform linux/amd64,windows/amd64` | ✅ **零代码改动**：`.deb` 7.3M / `.tar.gz` 7.4M / `.exe` 17.4M |
| E11 | mygo 与 purego 的依赖面 | `go list -deps ./cmd/leeway` | mygo **0**、purego **0**；只有 `cmd/leeway-gui` 拉进（19 个 mygo 包） |

E10 在 mygo **v0.3.4** 上复测；E1 / E11 在当前 `main` 上复验。产物截图与完整日志见调研分支。

---

## 2. 三个关键发现

### 发现 1：挡住 Android 的不是核心，是 UI 层的 mygo/purego（`实测`）

E1 + E11 证明：探测核心在 `CGO_ENABLED=0` 下直接编到 `android/arm64`，而 mygo + purego
**只被 `cmd/leeway-gui` 拉进来**。所以「Flutter 原生实现、不再依赖 mygo」这条路，顺带就甩掉了
purego 带来的那个 cgo 阻塞。剩下唯一需要 cgo 的地方是 FFI 桥本身（E4）——那是主动选择，
不是与 mygo「纯 Go 无 cgo」承诺的冲突。

### 发现 2：Android 上产品价值塌陷（`实测`，不是推断）

同一份 Go 核心、同一个函数，两个平台给出的答案：

| | macOS | Android |
|---|---|---|
| `运行平台` | darwin/arm64 | android/arm64 |
| `Home` | `/Users/<user>` | `/sdcard` |
| `config.Load()` | ✅ | ❌ `mkdir /sdcard/.config: permission denied` |
| **自动发现的 provider** | **4 / 7** | **0 / 7** |

两点必须分开看：

- `config.Load()` 失败是**工程问题**，可修：`Load()` 在文件缺失时会 `Save()`，而
  `os.UserHomeDir()` 在 Android 返回 `/sdcard`。给 Go 核心一个 app 私有目录即可（§8.3）。
- `0/7` 是**产品问题**：leeway 的核心卖点是「自动发现这台机器登了谁」，靠的是 macOS
  keychain、Chrome cookie 库解密、Cursor `state.vscdb`、`~/.codex/auth.json`、
  `~/.grok/auth.json`、kimi 桌面会话、官方工具。这些在 Android 沙箱下**全部不存在**。

### 发现 3：「出桌面端」是既成事实（`实测`）

E10：`mygo build -platform linux/amd64,windows/amd64` **零代码改动**产出可发布的
Linux `.deb` / `.tar.gz` 与 Windows `.exe`。所以原题里的 **mac 端早就有**，桌面还白送
Linux + Windows。**唯一真正新增的只有 Android。**

---

## 3. 成本盘点

### 3.1 UI 重写是主项（`实测`）

| 项 | 数量 |
|---|---|
| `internal/gui` | 18 个 `.go` 文件，5,702 行非测试 + 1,580 行测试 = **7,282 行** |
| `gui_test.go` | **52 个 Test 函数**、**32 张命名渲染快照**（35 个调用点） |
| 全仓顶层 Test 函数 | **421** |

快照与 `Tester.Menu()` / pinned clock / `meanDifference` 阈值那套自检，换栈后**无法迁移**，
只能重做。mygo 特有的坑（如「百分比高度按父容器 content box 解析」，见
[MYGO-NOTES.md](MYGO-NOTES.md)）会消失，但无障碍语义树、玻璃观感、键盘导航要重新踩。

### 3.2 桥接本身很小（`实测`）

PoC 的 C ABI 门面只有 **5 个导出函数**（`LeewayVersion` / `LeewayProviderCatalogJSON` /
`LeewayEnvironmentJSON` / `LeewaySetDataDir` / `LeewayFree`），Go 侧不到 200 行。
真正的成本在「谁持有数据、状态怎么同步、错误怎么过边界」，不在 FFI 语法。

### 3.3 体积：Flutter 引擎才是大头（`实测`）

| 产物 | 体积 |
|---|---|
| Flutter APK（fat，3 ABI） | 74.3 MB |
| ├ arm64：`libflutter.so`（引擎） | 11.2 M |
| ├ arm64：`libapp.so`（Dart AOT） | 4.2 M |
| └ arm64：`libleeway.so`（Go 核心） | 8.3 M |
| Flutter macOS `.app` | 41.2 MB |
| ├ `FlutterMacOS.framework` | 29 M |
| └ `App.framework` | 9.1 M |
| **mygo** GUI 二进制 darwin/arm64 | 24 M |
| **mygo** Linux `.deb` / `.tar.gz` | 7.3 M / 7.4 M |
| **mygo** Windows `.exe` | 17.4 M |

Go 核心在 Android 只占 8.3M，Flutter 引擎是它的 1.35 倍；macOS 上引擎 29M 比整个 mygo
二进制还大。

### 3.4 新增工具链（`实测`）

Flutter SDK 3.9G + JDK 17 309M + Android SDK（含 NDK 30 + 模拟器 + 系统镜像）12G。

---

## 4. 硬约束（`查证`，会决定方案边界）

| 约束 | 后果 | 来源 |
|---|---|---|
| AOSP SELinux `neverallow`：untrusted app 不得对 app home 内文件 `execute_no_trans`（targetApi>28） | **sidecar 子进程方案在 Android 上走不通**；但同一文件明确 `dlopen()` 仍允许 | [app_neverallows.te](https://android.googlesource.com/platform/system/sepolicy/+/refs/heads/main/private/app_neverallows.te) |
| Play「Device and Network Abuse」禁止从 Play 之外下载可执行代码（dex/JAR/`.so`） | 「下载 .so 再加载」不是合法分发路径；`.so` 必须打进 APK | [Play 政策](https://support.google.com/googleplay/android-developer/answer/9888379) |
| `targetSdk ≥ 35` 必须支持 16KB 页；2027-02-01 起不达标无法发版 | `.so` 要 `-Wl,-z,max-page-size=16384`（PoC 已做，E6） | [page-sizes](https://developer.android.com/guide/practices/page-sizes) |
| macOS 上 Dart VM 只能加载**已签名**的库 | 打进 `.app` 的 Go dylib 必须一起签名；PoC 的 dylib 是 `adhoc, linker-signed`，公证前要重签 | [dart.dev c-interop](https://dart.dev/interop/c-interop) |
| Flutter macOS 模板默认开启 App Sandbox | leeway 要读其它应用的凭据文件，得自行维护 entitlements | [Flutter macOS 构建](https://docs.flutter.dev/platform-integration/macos/building) |
| 上游 mygo 明确不做移动端 | 唯一移动端 PR #157（iOS-only）已关闭，维护者原话「i don't want to maintain ios/android support」 | [mygo PR #157](https://github.com/egoist/mygo/pull/157) |
| Wails v3 Android 标注 **experimental**，需 `CGO_ENABLED=1` + NDK | 「Wails 一套代码出双端」仍是 beta 押注 | [Wails v3 mobile](https://v3.wails.io/guides/mobile/) |

---

## 5. 踩到的坑（给后来者，`实测`）

1. **`flutter_test` 默认只有 Ahem 占位字体**：中文界面的 golden 是**一堵黑方块**。
   要在测试里用 `FontLoader` 把系统字体注册成 `Roboto` / `monospace` 才可读。
2. **Flutter macOS SDK 分 x64 / arm64 两个发行包**。装错（PoC 先装了 x64）会让整个工具链跑在
   Rosetta 下，`flutter test` 的 Dart VM 变成 x86_64，于是 arm64 的 dylib 加载不了。
   要么装 arm64 包，要么出 universal dylib（PoC 用 `lipo` 合并）。
3. **Go `c-shared` 的 install name 是裸名**（`libleeway.dylib`，无 `@rpath`），
   宿主必须自己修 rpath 或 install_name。
4. **`go run pkg@version` 会去查最新版**做 deprecation 检查；离线/受限环境下要改成
   `go build <pkg>` 走本模块的 module graph。
5. Android 模拟器 + `adb exec-out screencap` 可以**无头取证**，不需要真的开窗口。

---

## 6. 路线对照与建议

| 路线 | Go 核心 | UI | Android | 代价 |
|---|---|---|---|---|
| **A** 桌面继续 mygo（已出 macOS/Linux/Windows），不做 Android | 13.9k 全 | 7.3k 全 | ✗ | 最小 |
| **B1** 桌面继续 mygo + 移动端 Flutter 客户端，共享 Go 核心 | 13.9k 全 | 7.3k 桌面保留，移动端新写 | ✓ | 中 |
| **B2** 全部 Dart 重写（CLI + 桌面 + 移动同一套 Dart） | 全弃 | 全弃 | ✓ | 最大（§7） |
| **C** 桌面 + 移动一套 Dart UI，核心仍走 Go | 13.9k 走 c-shared + dart:ffi | 7.3k 全弃重写 | ✓ | 大 |
| **D** 换 Wails v3 | 13.9k 全 | 7.3k 弃，改 HTML/JS | ✓ 但 experimental | 中～大 |
| **E** 换 Tauri v2 | 不可直接用（Rust 重写或再挂 FFI） | 重写 | ✓ | 最大 |

> B2 与 C 的区别是「Go 核心留不留」；B1 与 C 的区别是「桌面留不留 mygo」。

### 推荐：路线 B1，且移动端产品定位要改

- **桌面**：`internal/gui` + mygo 一行不动，继续出 macOS/Linux/Windows 三端，
  `cmd/leeway` 的 CLI 契约不受影响。
- **移动**：新的 Flutter 工程，只依赖 `internal/*` 的探测核心（PoC 已证明能编到
  `android/arm64` 并经 `dart:ffi` 调用）。**不用** `internal/switcher`——
  `leeway use` 在非 darwin/linux 直接 `errUnsupportedOS`，且沙箱下没有可写入的对端。
- **产品定位必须改**：从「自动发现本机登了谁」改成「引导式粘贴凭据 + 额度看板 + 重置提醒」。
  竞品印证：官方 CodexBar iOS 版彻底放弃读本机凭据改走引导式登录；CodexBar-android 要求
  用户先在 macOS 上用 `security` / `cat` 抠出 token 再手工粘贴（见 [COMPETITORS.md](COMPETITORS.md)）。

### 什么情况下才该选 C / B2（把桌面也换掉）

同时满足前两条才建议：

1. 目标平台 ≥3 且其中 ≥2 个是移动端，且要求窗口与移动端长期同一套视觉/交互；
2. 愿意把 7.3k 行 Go UI（52 测试 / 32 快照）判为沉没成本，并接受 FFI 边界成为长期自研面。

若连 Go 核心也不要，选 B2 的额外门槛见 §7.6。

---

## 7. 追问：连 Go 核心也不要，全部 Dart 重写？

### 7.1 要重写多少（`实测`行数）

| 范围 | 生产代码 | 测试代码 | 合计 |
|---|---|---|---|
| `internal/gui`（UI） | 5,702 | 1,580 | 7,282 |
| `internal/*` 其余（核心） | 13,924 | 9,561 | 23,485 |
| `cmd/*` 入口 | 58 | 0 | 58 |
| **保留 Go 核心，只写 Flutter UI** | **5,702** | 1,580 | **7,282** |
| **全部 Dart 重写** | **19,684** | 11,141 | **30,825** |

**全量重写是只重写 UI 的 4.2 倍。** 核心 13,924 行里 `internal/provider` 占 7,108 行
（7 家、7 套协议：`opencodego` 1598、`grok` 1235、`kimi` 1214、`deepseek` 1126…）。

### 7.2 Dart 能不能做（`实测`，三个独立实现交叉验证）

把核心里最难的一块——Chrome macOS cookie 解密——用 Dart 复刻，并与独立参考实现对比：

```
Dart   pbkdf2 key     : d9a09d499b4e1b7461f28e67972c6dbd
Python hashlib（参考） : d9a09d499b4e1b7461f28e67972c6dbd   ← 一致

Dart   cbc ciphertext : 9f26ccc86235ad8fcf193f4e3a9a0b35f30f20b46313b0c9a0beb4d0e90cf428
openssl（参考）        : 9f26ccc86235ad8fcf193f4e3a9a0b35f30f20b46313b0c9a0beb4d0e90cf428   ← 一致

sha256(hostKey) 前缀  : ok
SQLite BLOB 端到端往返 : ok   (package:sqlite3, SQLite 3.53.4)
```

**逐位一致。** 核心其余部分 Dart 也都能做：

| 核心需求 | Go 现状 | Dart 等价物 | 状态 |
|---|---|---|---|
| Chrome cookie 解密 | 手写 PBKDF2 + AES-128-CBC | `crypto` + `pointycastle` | ✅ 已实测逐位一致 |
| 读 `state.vscdb` / Chrome DB | `modernc.org/sqlite` | `package:sqlite3`；Flutter 侧 `sqlite3_flutter_libs` 0.6.0 | ✅ 已实测（macOS） |
| keychain / `security` CLI | 28 处 `exec.Command` | `Process.run` | ✅ 直接等价 |
| grok 的 grpc-web + protobuf | 手写 `protobuf.go` / `grpcweb.go` | `protobuf` 6.1.0 / `http` | ✅ 包存在 |
| brotli / gzip | 标准库 + 三方 | `brotli` 0.6.0 / `archive` 4.3.0 | ✅ 包存在 |
| 并发拉取 7 家 | goroutine | `Future.wait` / isolate | ✅ 本来就 IO 密集 |
| `syscall`（全核心仅 5 处） | `ECONNRESET` / `SIGTERM` / `flock` | `SocketException` / `ProcessSignal.sigterm` / FFI `flock(2)` | ✅ 量极小 |

**没有哪一块是 Dart 做不到的。约束不是能力，是成本。**

> 未验证项（`推断`）：Android 侧 `sqlite3_flutter_libs` 只确认了发行包存在，未在设备上实测。
> 交叉验证全部在 macOS（darwin）上完成。

### 7.3 CLI 分发代价（`实测`）

| 方案 | 体积 | 启动（15 次均值） |
|---|---|---|
| Go CLI（现状） | 17 M 单文件 | 0.035 s |
| Dart AOT CLI（无 native 资产） | **5.8 M 单文件** | 0.037 s |
| Dart CLI + SQLite | 6.0 M 可执行 + 1.7 M `libsqlite3.dylib` = **7.6 M 目录** | 0.037 s |

Dart 反而**更小、启动一样快**。但有两个硬性差异：

- 只要依赖 `package:sqlite3`（Chrome / Cursor 都要），`dart compile exe` 会直接拒绝：
  `'dart compile' does not support build hooks, use 'dart build' instead`；
  必须走 `dart build cli`，产出的是**一个 bundle 目录**，不是单文件。
- 更要紧的是 `go install ./cmd/leeway` 这条现行安装路径**没有了**，要改成
  brew / scoop / Release 附件。CLI 是文档里的契约，改动要同步 README 与 `leeway help`。

### 7.4 关键：全量重写**不能**解决 Android 的产品问题

§2 的 Android 实测结果与语言无关：`config.Load()` 失败与 **0/7** 都是**操作系统沙箱事实**。
换成 Dart 写，一样是 0/7。**全量重写买到的是工程优雅，不是产品能力。**

### 7.5 它真正的价值：消除「两份实现」

若移动端走 Flutter，留着 Go 核心意味着二选一：经 FFI 共享（cgo + NDK + ABI 边界），
或在 Dart 里再写一份探测逻辑——**两份实现，每次上游 API 变动要改两处**。

`internal/provider` 是 7,108 行、7 套第三方协议，且上游会变。

> `推断`：本仓全历史提交数很少（样本不足以做变更频率统计），**无法用实测证明**它是否高频
> 变动。这一条是领域推断，不是测量结果。

| | B1：Go 核心 + FFI | B2：全部 Dart |
|---|---|---|
| 要写的量 | 5,702 行 UI | 19,684 行生产 + 11,141 行测试 |
| 语言/工具链 | 2 套（Go + Dart） | 1 套 |
| 长期接缝 | cgo + NDK + ABI 稳定性 + C 字符串 JSON | 无 |
| CLI 安装 | `go install` 不变 | 改成 brew/scoop/Release |
| 上游 API 变动 | 只改 Go 一处 | 只改 Dart 一处 |
| iOS | 需要另做 FFI 打包 | 顺带就有 |
| 现有 421 个 Go 测试 | 保留 | 全弃重写 |

### 7.6 建议

- **若 Android 只是桌面产品的附带客户端** → B1。花 5.7k 行换来一个窄 UI 最划算，
  核心与 CLI 契约一动不动。
- **若目标是真正的多端产品（尤其含 iOS）** → B2 成立，且「避免两份实现」是它最强、
  也最容易被忽略的理由。Flutter 引擎 11～29M 的体积与 4.2 倍的一次性重写，是为长期
  单一代码库付的价。
- **无论选哪个，先做移动端的产品定义。** 这件事没想清楚，B1 和 B2 都会做错。

---

## 8. 移动端降级实现：只查有 API key 的供应商

这是本调研给出的、**成本最低且已验证可行**的移动端形态。

### 8.1 逐家核实：谁真的能用「光一个 key」查到额度

判据不是 `AuthSpec` 的声明，而是 **fetch 路径 + 本仓自己的测试**（测试里只传
`Credentials{APIKey: ...}` 而不传 cookie / 文件，就说明这条路径自洽）。

| provider | 移动端模式 | 代码证据 | 手机上能做什么 |
|---|---|---|---|
| deepseek | `api-key` | `AuthSpec` 只有 `EnvAPIKey`；注释「API key 是唯一必需凭据；无 cookie、无账户」；测试只传 `Creds.APIKey` | 粘贴 key |
| commandcode | `api-key` | `AuthSpec` 只有 `EnvAPIKey`；`Authorization: Bearer <apiKey>`；测试只传 `Creds.APIKey` | 粘贴 key |
| opencodego | `api-key` | `Fetch`: `if req.Creds.APIKey != "" { fetchByAPIKey }`；注释「one key = one subscription, **no browser session required**」 | 粘贴 key |
| kimi | `api-key` | `Fetch`: `case apiKey != "": fetchCodeAPI(...)`；`GET {base}/coding/v1/usages` bearer；web token 只是可选 enrichment | 粘贴 key |
| cursor | `session-token` | `resolveToken` 接受 `Creds.APIKey`/`AuthToken` 且对粘贴值不做过期校验，但那是 Cursor **会话 token**，得先在桌面端抠出来 | 桌面端取 token 再粘贴 |
| grok | `desktop-only` | `AuthSpec` 只有 cookies + `~/.grok/auth.json`，无 env key；测试里 `APIKey` 出现 **0 次** | 不可用 |
| codex | `desktop-only` | `AuthSpec` 只有 `UsesCodexCLI`；token 来自 `~/.codex/auth.json` 的 `tokens.access_token`；端点是 `chatgpt.com/backend-api/wham/usage`（ChatGPT 会话）；测试里 `APIKey` 0 次 | 不可用 |

**4 家粘贴 key 直接用，1 家需桌面端导 token，2 家移动端不可用。**

> kimi 容易被误判为「只支持 web 会话」。实际上 `fetchCodeAPI` 是与 `fetchWeb` **并列的独立
> key 路径**（`internal/provider/kimi/kimi.go:138`），`KIMI_CODE_API_KEY` 单独就能用。

### 8.2 不用改 Go 核心：降级是自动发生的

`ResolveCredentials` 已经是「env → config」两级，provider 内部再自然分流：
移动端永远不填 `CookieHeader` / `CodexAuth`，于是 `canAttemptFetch` 对 grok/codex 返回
**false** → 显示 `not_configured`（正是想要的 UX），对 4 家 key 型返回 true → 走 key 路径。

**核心不需要加任何开关。**

### 8.3 唯一的硬阻塞已修好，且零改 Go

`config.Path()` 本就支持 `LEEWAY_CONFIG` 覆盖，`auth.CacheDir()` 支持 `LEEWAY_CACHE`。
桥接加一个 `LeewaySetDataDir(dir)` 把这两个指向 app 私有目录即可：

| | 配置路径 | `config.Load()` |
|---|---|---|
| 修前 | `/sdcard/.config/leeway/config.json` | ❌ `permission denied` |
| 修后 | `/data/user/0/<pkg>/files/config.json` | ✅ 无错误 |

**Go 核心一行未改。** 实测截图见调研分支 `out/poc_android2.png`（修前）、
`out/poc_android3.png` / `out/poc_android4.png`（修后，7 家移动端模式）。

### 8.4 降级把重写量砍掉多少（`实测`）

| 移动端需要的 | 行数 |
|---|---|
| `internal/provider` 顶层（接口 + registry） | 177 |
| 5 个 key/session 型 provider 包 | 4,765 |
| `internal/model`（JSON 契约） | 127 |
| `internal/netutil` | 45 |
| **合计** | **≈ 5,114** |

**不需要**：`internal/auth` 1,441（keychain / Chrome cookie / Cursor 会话）、
`internal/switcher` 553、`internal/history` 606、`internal/config` 350、
`internal/cli` 2,261、`internal/render` 776、`internal/credwrite` 101 ≈ **6,088 行**。

| 方案 | 要写/复用的核心量 |
|---|---|
| 只重写 UI（保留 Go 核心，全功能） | 5,702 行 UI |
| 全量 Dart 重写 | 19,684 行 |
| **移动端降级客户端** | **≈ 5,100 行**（Go 复用或 Dart 重写，量级相同） |

而且 **§7.2 里花力气证明的那些最难的东西，降级方案一个都不需要**：SQLite（Chrome cookie /
Cursor `state.vscdb`）、macOS keychain、Chrome 的 PBKDF2 + AES 解密——移动端全用不上。

### 8.5 一个真实的坑：cursor 在移动端是 error 而非 not_configured（`实测`）

`canAttemptFetch` 对 `spec.UsesKeychain` **无条件 return true**。cursor 带这个标志，
所以移动端会**总是尝试** cursor，失败后显示
`no Cursor token — set CURSOR_ACCESS_TOKEN, run leeway login cursor, ...` ——
状态是 **error**，不是 `not_configured`。

好消息：这个错误是**干净的登录提示**，不是裸的 exec 错误。`CursorKeychainTokenFor` 先
`LookPath("security")`，而 `IsKeychainItemMissing` 对 `exec.ErrNotFound` 返回 **true**，
所以 Android 上会正常降级穿过 keychain 分支：

```
PATH=/nonexistent
CursorKeychainToken -> token="" err=exec: "security": executable file not found in $PATH
IsKeychainItemMissing(err) = true   ← 被当成「没有这一项」，干净降级
```

处理方式：移动端 UI 把 cursor 归到 `session-token` 组（PoC 就是从 `AuthSpec` 推导的），
或给核心加一个「跳过 keychain」的小开关。

### 8.6 对路线选择的影响

降级方案的核心复用面里 **4,765 / 5,114 是 provider 协议代码——恰好是随第三方 API 变动的
那一部分**。这让「两份实现」的代价从 §7.5 的定性判断变成可计量的事实：上游每改一次协议，
两份都要跟。所以在本场景下 **B1 比 §7 时更站得住**。

反过来说，如果只做这 4 家 key 型 provider，Dart 重写也就是约 5,000 行的一次性投入，
换零 cgo / 零 NDK / 零 FFI。

### 8.7 仍需想清楚的产品问题

- **key 存哪**：`flutter_secure_storage` 11.2.0（Android Keystore / iOS Keychain）。
  **不要**复用 `internal/credwrite`——那是给桌面 CLI 写 DSH 配置用的。
- **iOS 同理**：cursor 的 `UsesKeychain` 在 iOS 上一样走不通，模式与 Android 一致。
- **定位要诚实**：只查有 key 的家，意味着移动端**完全没有** leeway 的招牌能力
  （自动发现本机登录态）。它是「跨端额度看板」，不是「leeway 手机版」。

---

## 9. 怎么复核

完整 PoC 在调研分支，切过去即可：

```sh
git worktree add .worktrees/ui-flutter-lab analysis/mygo-to-flutter
cd .worktrees/ui-flutter-lab
export GOCACHE=$PWD/.gocache          # 避开受限的全局构建缓存

# 1) Go 核心 -> Android .so（需 NDK）
export ANDROID_HOME=$HOME/Library/Android/sdk
bash lab/build-android-libs.sh        # -> out/jniLibs/{arm64-v8a,armeabi-v7a,x86_64}/libleeway.so

# 2) macOS universal dylib
CGO_ENABLED=1 go build -buildmode=c-shared -o out/libleeway.dylib ./lab/bridge
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -buildmode=c-shared -o out/libleeway-amd64.dylib ./lab/bridge
lipo -create out/libleeway.dylib out/libleeway-amd64.dylib -output out/libleeway-universal.dylib

# 3) 出 APK（.so 已在 android/app/src/main/jniLibs/）
export PATH="$HOME/development/flutter/bin:$HOME/development/jdk17/Contents/Home/bin:$PATH"
cd lab/flutter_poc && flutter build apk --release

# 4) 模拟器无头取证
emulator -avd leeway_poc -no-window -no-audio -gpu swiftshader_indirect &
adb wait-for-device && adb install -r build/app/outputs/flutter-apk/app-release.apk
adb shell am start -n dev.helixliu.leeway_poc/.MainActivity
adb exec-out screencap -p > ../../out/poc_android.png
```

§1 的 E1 / E11 复验（在任意 checkout 上都能跑）：

```sh
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build ./cmd/leeway    # E1：应 exit 0
go list -deps ./cmd/leeway | grep -c 'egoist/mygo'                # E11：应为 0
go list -deps ./cmd/leeway-gui | grep -c 'egoist/mygo'            # E11：应为 19
```

---

## 10. 实验件清单（都在调研分支）

| 路径 | 是什么 |
|---|---|
| `lab/FEASIBILITY.md` | 本调研的分支内版本（含更多过程细节） |
| `lab/bridge/main.go` | cgo C-ABI 门面，导出 §3.2 那 5 个函数 |
| `lab/build-android-libs.sh` | NDK 交叉编译三个 ABI 的 `libleeway.so`，带 16KB 页对齐 |
| `lab/flutter_app/main.dart` | 纯 Flutter 客户端源码（无 mygo 依赖），含 `dart:ffi` 绑定与移动端模式渲染 |
| `lab/flutter_poc/` | `flutter create` 工程，`.so` 已放进 `android/app/src/main/jniLibs/` |
| `lab/dart_spike/` | Dart 复刻 Chrome cookie 解密（PBKDF2 + AES-128-CBC + SHA256 前缀） |
| `lab/portcheck/` | 验证无 `security` 二进制时 cursor keychain 会干净降级 |
| `out/poc_android2.png` | 数据目录注入**前**（`config.Load()` permission denied） |
| `out/poc_android3.png` `out/poc_android4.png` | 数据目录注入**后**，7 家移动端模式实测 |
| `lab/flutter_poc/test/goldens/poc_overview.png` | macOS 侧 golden（4/7 自动发现） |

---

## 11. 未做与未知

诚实列出这份调研**没有**回答的问题：

1. **没有发出任何真实 provider HTTP 请求**。所有结论来自代码路径、本仓测试与离线数据，
   「4 家能查额度」指的是**代码路径可达**，不是「已在手机上成功拉到真实余额」。
2. **未在 iOS 上做任何实测**。iOS 的 FFI / 签名 / 沙箱结论全部是 `推断`。
3. **未在真实 Android 设备上跑**，只有 Android 16 arm64 模拟器。
4. **未评估 Flutter UI 与现有 mygo 观感的差距**。PoC 只有一个页面，玻璃材质、入场错峰、
   走势图、键盘导航、VoiceOver 语义树都**没做**，[GUI.md](GUI.md) 描述的那套视觉没有对照。
5. **139 人日那类工时估算未采用**。本仓可辩护的说法是「单人、一个季度以上」，
   不是「几周就能port完」；具体人日属判断而非测量。
