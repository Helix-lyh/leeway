# Pulse 美学参考（MyGo 实机）

> 目的：把 [Pulse](https://pulse.egoist.dev/)（egoist，MyGo 原生窗口，**无公开源码**）当成 leeway
> 窗口的**美学标杆**。结论只来自本机实机截图 + 二进制字符串，不以官网浅色营销稿为准。
>
> 截图目录：[`docs/ref/pulse/`](ref/pulse/)（2026-10-11，Pulse 0.1.11，深色模式）。
> 本机安装：`/Applications/Pulse.app`；偏好约在 `~/Library/Application Support/Pulse/`。

对照不等于照搬：Pulse 是系统监视器，leeway 是额度探测。学的是**空间分层、选中态、卡片与量的画法**，
不是点阵图或硬件页信息架构。

---

## 0. 先看图

| 文件 | 页面 |
|---|---|
| [`01-overview.jpg`](ref/pulse/01-overview.jpg) | 总览：3 列资源卡 + 下排组合卡 |
| [`02-processes.jpg`](ref/pulse/02-processes.jpg) | 进程：`Table` + 筛选 segmented + 底栏活值 |
| [`03-cpu.jpg`](ref/pulse/03-cpu.jpg) | CPU：满宽点阵主卡 + 核心条 + 活动卡 |
| [`04-memory.jpg`](ref/pulse/04-memory.jpg) | 内存：点阵 + 甜甜圈 / 压力环 / Swap |
| [`05-gpu.jpg`](ref/pulse/05-gpu.jpg) | GPU：点阵 + About / Engines 条 |
| [`06-disk.jpg`](ref/pulse/06-disk.jpg) | 磁盘：读写点阵 + 容量条 + 映像列表 |
| [`07-network.jpg`](ref/pulse/07-network.jpg) | 网络：上下行点阵 + 接口卡 + Top 用户 |
| [`08-energy.jpg`](ref/pulse/08-energy.jpg) | 能耗：电池环 + 功率点阵 + App/温度条 |
| [`09-about.jpg`](ref/pulse/09-about.jpg) | 关于本机（及部分详情页拼图） |
| [`10-energy-settings.jpg`](ref/pulse/10-energy-settings.jpg) | 右上角 **glass 设置 popover** |
| [`11-energy-detail.jpg`](ref/pulse/11-energy-detail.jpg) | 能耗细部：卡内填充条、状态 pill |

**不要用** `pulse.egoist.dev` 营销图当依据——那批是浅色、折线/面积图、灰 wash 选中，和实机不一致。

---

## 1. 空间骨架

```
Row
├─ Sidebar（glass / vibrancy，固定宽）
│   ├─ 概览 / 进程
│   ├─ 硬件 section（色块图标 + 名 + 右侧活值）
│   ├─ 系统 section
│   └─ 底栏：机型 / 芯片 / 运行时间（钉死）
└─ Column（主列，更深底）
   ├─ page head：标题 + 副标 + 时间段 segmented + 暂停/设置
   └─ Scroll：卡片 Grid / 卡栈 / Table
```

推断实现（无源码，二进制旁证）：`glass` / `Material`、`ScrollEdge`、`ui.Table` / `ui.List`、
元素 key 如 `card:` / `hero:` / `ring:` / `donut:` / `tray:` / `head:` / `page:`。
MyGo 伪版本约 `v0.3.8`（2026-10-10 tip）；窗口 prefs 含 `sidebarWidth`、`hideSidebar`、`page`、
`inspector`、`groupByApp`。

---

## 2. 可抄的美学规则（给 leeway 用）

### 2.1 三层灰，不靠描边

| 层 | 实机观感（深色） | 作用 |
|---|---|---|
| 窗底 | ≈ `#141416` | 主画布 |
| 卡面 | ≈ `#1f1f23` / `#26262b` | 圆角块，靠亮度差分层 |
| 图区内凹 | 略深于卡面 | 点阵/图坐在槽里 |

卡片之间是 **Gap + 填充差**，几乎无 1px 描边。可点卡右上角有 `>`，整卡是 hit target。

二进制调色板片段（旁证）：`#141416` `#1f1f23` `#26262b` `#1a1a1d`，强调 `#007aff` / `#0a84ff`，
语义色 `#ff3b30` `#28a745` `#ff9500` `#2fa8e0` 等。

### 2.2 侧栏选中 = 系统蓝 pill

选中项是 **实心圆角蓝条**（白字），不是中性灰 wash。未选中行右侧挂**活值**（`27%`、`8.83 MB/s`），
导航兼状态条。底栏机型信息小字、低对比，不抢导航。

> 与 HANDOFF §4.1「开源语料 0/7 用强调色选中」是**分叉**：那条来自 motrix 等开源样本；
> Pulse 是 egoist 自家成品的另一极。leeway 现状跟 motrix；若要抬品质，Pulse 的蓝 pill +
> 行内活值是优先对照对象（改之前先量、先证伪）。

### 2.3 页头承载控件，正文只放答案

时间范围（1m / 5m / 15m / 1h）用 **segmented pill**；暂停、设置是安静图标按钮。
设置是 **glass popover**（能透过看到后面的点阵），不是整页设置墙。

### 2.4 量：色填充说话，不要灰框

- **条**：深灰圆角底槽 + 彩色填充，**无描边**；核占用、容量、App 能耗同一路。
- **环 / 甜甜圈**：自绘（`ring:` / `donut:`），中心大数字或短状态。
- **阈值**：高占用用**字色**（橙黄）提示，不是给整行涂底。

### 2.5 签名可视化：点阵（学精神，不必然抄形态）

时间序列是固定网格上的圆点：先铺极淡底点，再按采样点亮彩色点（可叠两色）。
总览卡里缩成 spark-dot，详情页拉满宽。实现上是 `Box.Draw` 网格画圆，不是折线 Path。

对 leeway：**学的是「量有自己的纹理、不靠边框」**；额度走势是否改成点阵另议，默认继续现有面积/折线即可。

### 2.6 类别色当锚点

CPU 蓝、内存绿、GPU 粉红、磁盘橙、网络青、能耗黄/绿——图标底、点阵、条填充共用一类色。
leeway 已有 provider 色时，保持「一类别一色贯穿图标与量」，避免每块另起一套。

---

## 3. 页面模式速查

| 模式 | 结构 | 代表图 |
|---|---|---|
| 总览 bento | 上 3×2 资源卡（大数字 + 副标 + 底点阵 + `>`）+ 下组合卡 | `01` |
| 详情主卡 | 满宽 hero + 点阵；下 2～3 列子卡 | `03`–`08` |
| 密度表 | 页头搜索/segmented；`Table` 可展开父子；底栏汇总 | `02` |
| 关于 | 大图 / 规格多列，少图表 | `09` |
| 浮层 | glass popover，背景不卸掉 | `10` |

---

## 4. 和 leeway 的对照（决策用）

| 点 | Pulse 实机 | leeway 现状（HANDOFF） | 参考含义 |
|---|---|---|---|
| 卡片分层 | 三层灰、无描边 | 画布 + 卡 + 柔投影，无描边 | **已对齐精神** |
| 侧栏选中 | 蓝 pill | 中性灰 wash（跟 motrix） | 品质抬升时优先对照 |
| 侧栏活值 | 每行右侧即时值 | 已有「每家还剩多少」方向 | 继续强化即可 |
| 额度/进度条 | 深槽 + 色填充、无边 | Pulse 软槽（`p.track` + 填充） | 已对齐：0% 仍见虚影，无描边 |
| 时间序列 | 点阵 | 折线/面积 | 精神：纹理服务数据；形态可不抄 |
| glass | 侧栏 + 设置弹层 | 标题区 Material；ScrollEdge 未上 | 弹层/叠层场景再评估 |
| 线密度 | 几乎无满宽装饰线 | core5 装饰线已压低 | **继续压** |

---

## 5. 复拍（本机有 Pulse 时）

```sh
# 偏好与窗口
open -a Pulse
# 截图保存进仓库参考目录（手动或系统快捷键），命名延续 01-… 序号
# 二进制旁证（无源码时）
strings /Applications/Pulse.app/Contents/MacOS/Pulse | rg 'card:|donut:|ring:|glass|ScrollEdge|#007aff'
```

改 leeway 版式前：打开本目录对应 JPG，和 `LEEWAY_GUI_SNAPSHOT` 快照并排，先量再改。
