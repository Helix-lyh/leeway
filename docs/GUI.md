# leeway 窗口界面

> 目标是「CLI 干完的事，窗口里也能干，且不用背参数」：探测核心、配置、缓存、会话目录全套复用 `internal/*`，
> `internal/gui` 只做编排与绘制。UI 用 [MyGo](https://github.com/egoist/mygo) 的原生
> `ui` 包（GPU 绘制、纯 Go、无 cgo），所以是一个单文件二进制、没有 Node 也没有 WebView。

有意的例外：只配 key 的三家（deepseek / opencode-go / commandcode）不提供「让本机用它」——本机没有它们的
官方客户端可切；把 key 写进 DSH 那件事留在 CLI 的 `leeway use <provider>`。

## 进入编排

窗口打开时（`Open()` / `OpenSettings()`）记下时刻，第一帧里各块按顺序入场：`arrival.hold()`
每包一块就把 `index` 加一，第 i 块比前一块晚 `arrivalStep`(55ms) 起步，用同一个 `easeGlide` 上浮
10 DIP 并淡入，`arrivalDur`(400ms) 走完。设置窗口只有一块内容，整体入场，不做逐块错开。

块装进入场自己的 holder 里，而不是直接改块本身：卡片带 `panelMotion`，它进入时会**写**元素的
opacity，把入场设的透明度盖掉（第一版就是这么静默失效的）；另外 `Margin(0,0,x,0)` 是下边距，
只把后一块往下推，不会把本块抬起来，要用上边距。时间取 `a.now()`，也就是窗口自己的钟。

`c.Preferences().ReduceMotion` 为真时整条路径原样返回。窗口之外的渲染（headless 测试、快照）没有
`arrival.at`，`hold()` 不加任何样式，所以快照永远是"静止态"。`TestTheWindowEntersInOrder` 把钟
钉住再把 `arrival.at` 设成 150ms 前，就能稳定拿到一张**入场中途**的帧（`window-entrance.png`）。

## 可复现的渲染

渲染里所有"现在"都走 `App.now()`（`a.clock` 为空时就是 `time.Now`），`agoText` / `resetLabel` /
`resetMeta` / `windowMeta` 由调用方把时刻传进去。测试把 `a.clock` 钉在 `pinned` 上（一个固定日期，不是 `time.Now()`，否则跨次
运行的时间戳还是会变），fixture 也用同一个时刻，于是同一份代码两次跑出的快照几乎逐像素相同——在此之前 5 张图每跑一次都不一样，根本
分不清是改坏了还是时钟走了一格。钉住之后还暴露出一个假断言：fixture 的 `reset = now+52h` 原本因为
毫秒漂移显示成"2 天 3 小时"，实际正好是"2 天 4 小时"。快照不是逐字节可复现的：还在跑颜色过渡的
底部一条带会有最大 2/255 的差，所以比对用 `meanDifference` 的阈值，不要断言字节相等。

## 无障碍

屏幕阅读器看到的是一棵树：页面标题是 `RoleHeading` + `Level(1)`，卡片标题是 `Level(2)`，
所以能按标题跳读；`mark()` 里的图标本身是装饰（MyGo 里没有 Label 的图片不进树），provider 名字只念一遍；工具栏的纯图标
按钮都有 `Label`（`iconButton()` 默认要求一个）。切页时 `Announce` 页面名（和 MyGo 的 Router
一样），探测结束念一行结果，toast 由 MyGo 自己念——我一开始又手动念了一遍，测试里听到两遍才
发现是重复的。

## 入口

```
cmd/leeway-gui/main.go     # mygo.App：名字、版本、单实例锁、菜单、WhenReady(App.Open)
internal/gui/              # 全部窗口代码
```

`cmd/leeway` 一行没动：CLI 与 GUI 各自装配同一个 provider registry，共用 `internal/config`、
`internal/fetch`、`internal/history`、`internal/auth`、`internal/switcher`。

```
internal/gui/
  app.go          # App 状态、settings（gui.json）、刷新编排、provider/account 视图模型
  view.go         # 主窗口：标题栏、侧栏、页面路由、菜单、快捷键
  page_overview.go / page_provider.go / page_matrix.go / page_history.go / page_start.go   # 页面
  trend.go arrival.go   # 走势与用满预测、入场动效
  settingsview.go # 设置窗口（通用 / Provider / 数据）
  prompt.go / promptview.go   # 登录、导入 cookie、会话文件、新账号，以及确认框
  ops.go          # 登录/退出/use/启用等动作，落在共享的 auth/config/credwrite 上
  theme.go icons.go charts.go format.go   # 设计系统、图标、点阵、文案格式
```

## 设计系统（`theme.go`）

窗口是 macOS 26 那种「玻璃 chrome + 实体内容」的层次，规则写在 `theme.go` 一处，页面只调：

- **光**：窗口底是 `canvasTop → canvas` 的竖向渐变（顶部带一点强调色），玻璃按 `glass.Glass{}`
  在它上面结霜。油漆层不能盖内容：MyGo 的 `Absolute()` 会吃掉点击，所以底色是根容器的
  `LinearGradient`，不是绝对定位的覆盖层。
- **面**：`shell()` 是双层的「托盘」——外层 bezel（3px、细边、柔和投影）+ 内层表面，
  悬停时两层一起变亮，用 `ElementTransition` 过渡 160ms。`card()` 就是它。
- **字体**：六个尺寸（40 主数字 / 25 页标题 / 21 卡片数字 / 13 正文 / 12 次要 / 11 标签），
  数字一律 `FontFeatures("tnum")` 等宽并收紧字距，标签加 0.7 字距，路径用 `monospace`。
- **文字**：时间和路径都按中文说。倒计时是「2 天 3 小时后重置」，不是 CLI 的 `2d 03h`；
  路径经 `shortenPath()` 处理（家目录变 `~`，太长就留最后一两段），**文件名永远看得见**——
  设置页和关于页都不再出现四个一模一样的 `…/TestSetting…`。设置页里点路径即复制完整路径。

- **身份**：`mark()` 显示 provider **自己的图标**，不是我们画的字母：七家各自发布的图标
  （`internal/gui/icons/`，来源见 `icons/SOURCES.md`）统一裁成圆角方块嵌进二进制，右下角小点带
  状态。七块图标要像一排：都是**深色托盘 + 浅色字形**，四家发布的本来就是这种，另外三家
  只发布字形（DeepSeek 的蓝鲸、OpenAI 的结、opencode 的终端），脚本把字形抠出来按同一比例
  缩到 0.62，窗口用 `palette.markTile` 补同一块近黑托盘，所以深浅主题下这排都一样，
  `mark()` 只再补一层柔和阴影。某个 provider 没有图标时退回「专属色相 + 首字母」，而且
  `TestEveryProviderHasItsOwnIcon` 会让这种退路没法悄悄发生。
- **导航只列在用的**：侧栏「监控」里只有**启用且有订阅**的 provider（探测有结果，或本机有它的
  凭据）；行尾的数字是订阅数，一家一个也要写出来——空着读起来像"不知道"，不像"一个"。
  一家都没配时那里写一句「配置过的 provider 会出现在这里」。没配置的家不是一页要反复
  路过的导航，而是「开始使用」里读一次就够的一行。
- **历史**：一家 provider 一个托盘，里面每条序列一行（标题 + 值 / 40px 走势 / 一句说明），
  而不是一条序列一张大卡——provider 一多，卡片墙就没法比较。窗口在序列期间重置过，就在图上
  那个位置画一条竖线，说明里写「期间重置 N 次（图上的竖线）」，让标记有人解释。金额序列没有
  级别，走势和它的条一样用强调色。
- **布局高度自己拿主意**：标题栏下面那一行的高度是**算出来的**（`c.Size()` 减
  `titleBarHeight()`），不是交给 `Fill()` / `FillHeight()` 去猜。百分比高度会一路向上找到窗口
  那一层，于是滚动容器比看得见的区域高出正好一个标题栏，`maxScroll` 就少了这一段——页面拉到底
  还差一截（凭据位置那张卡就是这样被切掉的）。`TestTheEndOfALongPageIsReachable` 盯着这条：
  长页面滚到底，最后一行必须在滚动区自己的下边距之上。
- **数据**：`bar()` 是圆角轨道 + 渐变填充，首次出现时从 0 缓入；走势图用 1px 列拼出连续线
  （`Painter.Line` 只能画横竖线），线下一层很淡的平铺色，形状交给线而不是色块。金额窗口
  没有级别，条用强调色。主角卡的走势带一句话说明覆盖范围：「近 11 次探测 · 覆盖 3 小时 20 分 ·
  较上次 +6%」。图是按序列自身的最小/最大值铺满的，所以说明里必须写出两端的值
  （`18% → 78%`），否则「涨了两个点」和「涨了四十个点」会画成同一张图。重置倒计时留在上面
  那行，同一件事不说两遍；provider 页是细节页，改用绝对时刻加同样的区间。
- **动作**：`iconButton()` 是圆形图标按钮；表单一律「标签在上、控件在下」，标题栏方向由 `label()` 承担。
  主题设置不是一个下拉，而是三张**用它自己的配色画出来的小窗**（跟随系统那张是浅到深的渐变），
  选中的那张套强调色描边——看一眼就知道选哪个。
- **数字**：数值和它下面的条共用 `numberMotion`（520ms），刷新后是数字与条一起走到新值，
  不是条动、数字硬跳；`ui.Animate` 在首帧直接返回目标值，所以第一次显示不会假装从 0 数上来。

- **动效**：页面切换只淡入（`pageMotion`），卡片自己上浮 8px 落位（`panelMotion`），
  悬停由 `cardMotion` 收色；曲线是自定义 `easeGlide`（重减速）。页面负责淡、内容负责升，
  两层不互相打架；桌面开了「减弱动态效果」时 MyGo 自动跳过。

## 页面

| 页面 | 内容 |
|---|---|
| 总览 | 最紧张的一家占一张「主角卡」（大号数字 + 长条 + 历史点阵），其余成网格；失败/未配置/停用的进「需要处理」清单，一行一家。什么都没答上来时（首次运行）总览换成「开始使用」引导 |
| provider（单账号） | 身份、凭据来源、每个窗口的明细与历史点阵、花费、缓存/配置/会话路径，以及登录/退出/操作菜单（菜单内容由 provider 声明的凭据类型决定，见下） |
| provider（多账号） | 一列一个账号的额度矩阵：窗口做行、账号做列，数字 + 长条 + 重置时间同排，账号身份（名字、邮箱、套餐、状态）在列头；预测与重置提示按账号署名 |
| 历史 | `history.Collect` 出来的近 N 次探测点阵（每个 provider × 窗口一条），加变化量与「什么时候会用满」；还没探测过时是一张「还没有历史点」的引导卡 |
| 关于 | 版本、构建、配置/缓存/历史路径，复制 JSON |

凭据入口按 provider 自己声明的类型长（`AuthSpec`，不另立一张表）：官方 CLI 能登录的（cursor / grok /
codex / kimi）按钮写「登录」，只有 key 用来查额度/余额的（deepseek / opencode-go / commandcode）写「配置 key」；
菜单里出现的是这家真读的凭据——没有 cookie 这个来源就不会出现「从浏览器导入 cookie…」。`让本机用它`
只在真能切的时候出现：这家要有本机官方客户端（cursor / grok / kimi），而**这个账号**要有能写回去的会话
（cursor 得有 access + refresh 的私有会话，grok 得有完整的官方会话文件，kimi 得有会话文件）——只有 key
或只有 cookie 的账号不会出现这一项，codex 明确不写官方 `~/.codex`，key 类的 provider 本机也没有客户端可切。

多账号那家是**比大小**，不是**看细节**：四个账号、每账号三个窗口要在一屏里比出"谁快满了"，
一张账号一张卡做不到。所以多账号时换成矩阵——列是账号、行是窗口，同一行横向比，数值就是答案；
单账号仍然走整张卡（点阵、花费、说明都留得住）。两列之间是一条竖线，再窄也有：每一行都画，
上下接起来就是一条贯通的线，账号不会糊成一片。窗口标题当行头放在第一列，所以每条信息都在
自己那一列里——包括预测句，它只留"谁、还有多久用满"（涨速那句留在 provider 自己的页面上）。
列头带名字和邮箱，是因为同一家的几个账号只看数字分不清是谁。

侧栏底部是「数据」分组（历史、关于），上面是「监控」分组（总览 + 每个 provider）。
卡片右上角的圆形箭头、侧栏条目都能进 provider 页。

### 首次运行

一次探测都没成功、也没有失败记录时，总览给的是「开始使用」而不是「需要处理」——此时那张清单
就是同样的七家，列两遍没有意义。引导一行一家，说的是**这台机器上**这一家缺什么：

- 凭据来源**看得见才说**：环境变量、配置文件里的 key、`auth.json` 之类的会话文件、cookie 缓存，
  都要先确认存在；`UsesCodexCLI` / `LocalAuthFiles` 会去 `codex.AuthFilePath()`、
  `provider.LocalSessionPath()` 指到的地方 `stat` 一下。会话文件不在，就说「还没有 codex 会话。
  用官方 codex 登录，或导入 ~/.codex/auth.json。」——不能替用户假定它已经登录了
- 已经有来源的那家显示「已找到凭据」徽章和来源句，按钮从「登录」变成「刷新看看」
- 每行的主按钮取 `loginMethods(id)[0]`，也就是 `leeway login` 对该家的第一条路；其余路径在
  provider 页
- 已停用的一家显示「已停用，不参与探测。」和「启用」

## 变化与用满预测

历史点和走势图回答"发生过什么"，另外两句话回答"接下来会怎样"：

- 每行数值旁的变化量：`▲ +6%` / `▼ -2%` / `持平`。箭头带方向，颜色留给数值本身，不给涨跌上色
  （用量上涨不是"好"，下降也不一定是"坏"，没有一种颜色对两者都成立）
- **用满预测**：`最近 2 小时每小时涨 25%，约 1 小时 12 分后用满；重置还有 2 天 4 小时`。
  `burnOf` 需要两半才说话——这一段的速度，和这一窗的重置时间：

  - 只读**当前这一段**（`seriesLeg`）：重置会开新的一段，跨过重置算斜率会把"重置造成的暴跌"
    当成历史最快下跌
  - 至少 3 个点、跨度至少 10 分钟：两点一分钟内的跳变可能是抖动，宁可不说
  - 重置先到就不说（重置早就没用满，没什么可喊的）；已经满了也不说（进度条已经在说）
  - 同一句话出现在总览主角卡、provider 页窗口行、历史行，用词一致
- 出现预测时还会**主动**弹一条 toast（标题就是那句话），这是全局唯一不等用户来问的消息。
  同一个窗口只提醒一次（`burnToast` 记住上次点名的窗口），否则每分钟自动刷新的探测会每分钟弹一遍

## 刷新与状态

- 打开窗口：`settings.OnOpenFetch` 为真时探测一次（读 60s 缓存，`fetch.Options{UseCache:true}`）
- 自动刷新：按 `IntervalSec`（0 = 手动），心跳线程每秒只更新相对时间
- 手动刷新：标题栏按钮 / `⌘R` / 菜单「刷新」→ `UseCache:false`
- 探测结果：`history.LoadLastSeenBefore` → `Attach` → `Record` → `Collect(HistoryN)`，
  和 `leeway usage --last N` 同源
- 结果由 `App.apply` 交回主线程（`Window.Update`）再重绘；没有窗口时直接执行，所以逻辑可脱离窗口测试
- 失败时按 `NotifyOnError` 发系统通知，窗口里用红色行给出原因

## 设置

`~/.config/leeway/gui.json`（0600），与 `config.json` 分开存放——CLI 的 `config.Save()`
只认自己的字段，GUI 的设置放进去会被下一次 CLI 写配置抹掉。

```json
{
  "intervalSec": 60,      // 0 = 只手动刷新
  "timeoutSec": 15,       // 等价 LEEWAY_TIMEOUT
  "historyN": 24,         // 历史页点数
  "theme": "system",      // system / light / dark
  "lastPage": "overview", // 上次停留的页面
  "onOpenFetch": true,
  "notifyOnError": true
}
```

探测超时通过 `LEEWAY_TIMEOUT` 传给 `fetch`；主题通过 `mygo.Theme.SetSource`；
开机自启通过 `mygo.App.SetOpenAtLogin`（未打包运行时不生效）。

## 构建与打包

```sh
go run github.com/egoist/mygo/cmd/mygo@v0.3.4 build     # .app + .dmg
go run github.com/egoist/mygo/cmd/mygo@v0.3.4 build -skip-dmg
```

`mygo.json` 里的 `main` 必须写成 `./cmd/leeway-gui`：MyGo 用 `go list` 解析它，
`cmd/leeway-gui` 这种没有 `./` 的写法会被当成标准库路径。图标取 `resources/icon.png`，
构建器自己生成 `.icns`。DMG 需要 `hdiutil` 挂载磁盘映像，在受限沙箱里会失败，`.app` 不受影响。

图标由 `resources/icon.py` 画出（`python3 resources/icon.py`，只需要 Pillow）：1024 画布上一个
824 的 superellipse 主体（指数 5，Apple 的方圆），紫罗兰渐变，中间一条配额条——用掉的一段是白色，
剩下的一段是凹槽。**图标画的就是窗口里那个东西**：总览主角卡上的那条 bar。

签名是 ad-hoc（没有 Developer ID），所以 `spctl -a` 会拒；本机编译、本机安装照常运行。

## 验证

```sh
go test ./internal/gui/                 # 无窗口：ui.NewTester 渲染 + 交互，临时目录里跑配置
LEEWAY_GUI_SNAPSHOT=/tmp/shots go test ./internal/gui/   # 额外把每一帧写成 PNG
LEEWAY_GUI_CAPTURE=/tmp/window.png open build/darwin-arm64/leeway.app   # 真窗口截一张图后退出
```

- `ui.NewTester` 渲染真实帧、能点、能输入、能读回文字，所以页面结构、导航、表单都能测
- `LEEWAY_GUI_SNAPSHOT` 让测试把帧写成 PNG，便于人眼过一遍版式
- `LEEWAY_GUI_CAPTURE` 是打包后的自检：窗口画完一帧后 `Window.CapturePage` 存图并退出，
  在没有人在屏幕前的机器上也能确认「真的画出来了」
- 测试一律通过 `LEEWAY_CONFIG` / `LEEWAY_CACHE` / `LEEWAY_SESSIONS` 指向临时目录，
  不会碰用户的真实配置与凭据

## 边界与现状

- 当前只有窗口进程：关窗即退出，没有菜单栏常驻或后台守护。这是现状，不是产品禁令
- 界面文案中文；provider / 窗口名保持 registry 里的原文
- 不显示密钥内容，只显示凭据来源；`复制 JSON` 与 CLI 的 `usage --json` 同一份契约
- 数字文案不替 provider 下结论：模型只给了「值 + 上限」，界面就说「共 $20.00」，
  不写「已用」或「剩余」（balance 窗口和 spend 窗口的语义只有 provider 自己知道）
