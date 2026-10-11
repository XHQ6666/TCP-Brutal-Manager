# TCP Brutal Manager：执行与行为 SOP

版本：2026-10-12。依据：`TCP-Brutal-Config-Go-Plan.md` 中已确认约束。

本 SOP 描述待实现版本的操作流程和验收行为，不代表已有 Binary 或已完成 Linux 实测。开发仍按计划 → 实现 → 核对 → 编译 → 发版推进，各阶段分别确认。

## 1. 使用范围与执行前准备

Manager 通过官方 `brutalctl` 集中管理配置、DNS、规则更新、归属记录及持续维护。没有 `interface` / `table` 的普通路由由 brutalctl 管理；指定任一项时，Manager 通过 Netlink 维护已有策略 Route 属性。

执行前确认：

1. 使用 Linux，目标网络命名空间正确；首版支持基线为 TCP Brutal v2.0.1。
2. 内核模块已加载，`/proc/net/tcp_brutal/rules` 可读取；修改命令具备对应权限。
3. 官方 brutalctl 及其依赖的 iproute2 可执行，版本兼容性已核对。
4. 配置文件可读、JSON 有效，业务 `rules` 非空。需要指定策略路由的 Prefix 必须存在精确 Route。
5. 默认日志启用时确认日志路径可写；关闭文件日志使用 `log.enabled=false`。
6. 保留 `/var/lib/tcp-brutal-config/` 中的归属记录、初始化标记及恢复阶段记录，不将它们当作可随意删除的 DNS 缓存。

SOP 命令以实际 Binary 路径变量表示：

```sh
manager_bin=/usr/local/bin/tcp-brutal-config
manager_config=/etc/tcp-brutal-config/config.json
```

如果 Binary 改名或换位置，只修改 `manager_bin`。Help 的程序名来自实际调用文件名；配置、日志和状态目录不随改名改变。示例应在具有适当权限的 shell 中执行。

## 2. 配置输入与默认行为

### 2.1 Rate、CIDR 与排序

- Rate 为数字或字符串；无单位默认 Mbps，支持 bit 速率单位 bps / Kbps / Mbps / Gbps，不支持 Byte 单位。
- 精确换算后必须为正整数 Mbps，再将整数传给 brutalctl；例如 200、"200"、"200Mbps" 等价，"2.5Gbps" 为 2500；"2.5Mbps" 不符合整数要求。
- `ip` 为纯地址；`cidr` 是整数，IPv4 默认 32、IPv6 默认 128，显式 0 有效；先清除主机位。
- Domain 的 cidr 支持数字或 `{"v4":24,"v6":64}`，缺少地址族字段时该族使用最大长度。
- 最终 Desired：IPv4 在前，IPv6 在后；各族按网络地址数值升序，同地址按 CIDR 长度升序。Current 不排序。
- 去重保留全部来源；相同 Prefix 参数冲突隔离所有相关规则。不同 Prefix 可重叠。

### 2.2 DNS

- 无 DNS 服务器配置时使用 System；配置了服务器时，无匹配 DNS 选择规则使用首项。
- DNS Rule 的 resolver 引用唯一 name；domain 支持字符串或数组。full / domain / keyword / regexp 只用于 DNS 选择，无前缀为精确匹配，首次命中优先。
- fallback 默认 false；显式启用时解析器故障重试耗尽后按服务器数组顺序尝试其他项。每个 Resolver 最多三次尝试，间隔三秒；NXDOMAIN / NODATA 不触发 fallback。
- 有效结果只有一个地址族有地址时正常应用；成功整体无地址时清理该来源不再需要的旧状态。任一地址族最终查询故障时保护旧规则，不应用半份解析结果。
- DoH address 填域名，HTTPS 固定；path 默认 /dns-query，port 默认 443。DoT / DoQ 默认端口 853。

### 2.3 Settings 默认值

```json
{
  "settings": {
    "dns": {"query_interval": "60s", "timeout": "5s"},
    "config": {"debounce": "500ms", "rescan_interval": "60s"},
    "route": {
      "debounce": "500ms",
      "resync_interval": "60s",
      "operation_timeout": "10s"
    },
    "brutal": {"poll_interval": "30s", "operation_timeout": "10s"}
  }
}
```

Settings / 模块省略、空对象或 null 使用默认值；单字段缺省、null、空白使用默认值。非空值必须是正数 Duration。每次重载从默认值覆盖本次字段，删除字段恢复默认值。

## 3. 一次性检查与执行

### 3.1 查看帮助与当前状态

```sh
"$manager_bin" -h
"$manager_bin" help apply
"$manager_bin" -v
"$manager_bin" list
"$manager_bin" status -c "$manager_config"
```

Help / Version 不加载配置、不查 DNS、不要求 root 或模块。List 只读 Current，不要求配置。Status 展示配置、DNS 设置、实际规则、受管策略路由和同步摘要，不修改系统；其 DNS 刷新及退出码细则在实现前需与 Check / Diff 一致化，不能把旧摘要视为实时查询结果。

### 3.2 校验与预览

```sh
"$manager_bin" check -c "$manager_config"
"$manager_bin" diff -c "$manager_config"
"$manager_bin" apply -c "$manager_config" -n
```

Check / Diff / Apply Dry Run 均以本次实时 DNS 结果生成 Desired，不使用跨执行 DNS 缓存。输出有效规则、错误条目、Brutal 内容及顺序差异、策略 Route 差异、委托官方路由操作、旧状态保护和异常恢复范围。

Dry Run 不执行 Add / Update / Delete / Flush / Netlink 修改，不写归属或恢复记录。文件日志仍遵循配置。

预览只代表当时结果，不作为后续 Apply 的固定授权快照；真正 Apply 会重新查询 DNS、重新读取实际状态。

### 3.3 执行 Apply

```sh
"$manager_bin" apply -c "$manager_config"
```

执行顺序：

1. 全局校验及 Logger 初始化，逐条强类型解析。
2. 系统修改锁外查询 DNS、生成候选及来源。
3. 获取修改锁，重新读取 Brutal、Route、管理记录；冲突检测、去重、数值排序、Route 安全校验。
4. 生成最新计划；正常 Apply 不全量 Flush。
5. 持久化操作意图，逐条新增 / 原位更新规则及对应策略 Route。
6. 回读验证。Domain 新地址全部成功之前，保留成功新项和整组旧项。
7. 对成功更新组及明确删除来源清理旧受管规则；共享来源仍需要或保护的条目保留。
8. 按写入值匹配清理不再需要的 Go Route 属性，最后回读并保存实际结果。

无差异不修改。仅策略 Route 差异时只修复属性。普通路由由 brutalctl 自行处理；它的错误可能发生在规则已写入之后，应报告实际部分完成状态。

## 4. 规则及路由维护边界

| 配置 | 行为 |
|---|---|
| 无 table / interface | 普通路由交由 brutalctl 管理 |
| 仅 interface | 各表寻找 Prefix 与出口接口唯一精确匹配，确定表号 |
| 仅 table | 指定表寻找 Prefix 唯一精确匹配，读取出口接口 |
| 两者都有 | 精确定位并验证表、Prefix 和接口 |

没有唯一匹配或只有覆盖网段 / 默认路由时跳过该规则，不创建 Peer Route，不改变 ip rule。

lock 默认 true：规则与 Route 都锁定。lock=false：规则使用 nolock，Route 为 congctl brutal，清除仅对应拥塞算法的锁位。其他属性及 metrics 锁位保留。

允许覆盖策略 Route 现有拥塞算法；不承诺恢复旧算法。每次属性修改或清理前重读最新 Route，写后验证；外部竞争导致身份变化或验证不一致时暂缓该条。

普通 Apply 保留外部 Brutal 规则：同 Prefix 参数相同可复用但不认领，不同则跳过配置规则。策略操作若 brutalctl 的 noroute / del 会删除外部 proto 233 Route，则跳过对应规则。

## 5. Run 持续维护

```sh
"$manager_bin" run -c "$manager_config"
```

启动立即同步，仅允许一个 Run 实例。事件与定时器通过一个串行协调器合并处理，不并行修改。

| 触发源 | 动作 |
|---|---|
| DNS query_interval | 重新查询记录，变化后更新对应规则 |
| 配置目录事件 | 去抖、加载并校验新快照；相关 Domain / Resolver 变化触发解析 |
| 配置 rescan_interval | 内容核对兜底，不无条件重新查 DNS |
| Route / Link Netlink 事件 | 重定位及修复 Go 管理的策略 Route，不无条件查 DNS |
| Route resync_interval | 全量核对及订阅恢复兜底 |
| Brutal poll_interval | 读取受管规则状态，修复差异，不无条件查 DNS |

Run 非 DNS 事件使用本进程最近成功结果；重启仍重新查询。DNS 和 Brutal 不当作具有可靠原生变更通知。

新配置全局非法、文件暂时不可读或新 Logger 初始化失败：保留最后有效配置及 Logger。单条错误接受快照但跳过该条。Logger 准备成功后才切换；运行中全局故障暂停本次修改并报告，后续按对应触发重试。

Run 不支持 -n，预览用 Apply -n。SIGINT / SIGTERM 停止查询、监听和后续调度，已有状态保留，不自动 Flush。

需要 systemd 时按计划安装 Type=simple 服务，ExecStart 使用真实 Binary 和配置路径；Restart=on-failure。Tailscale 使用场景增加相应启动顺序。

## 6. 主动 Flush 与停止维护

要持续保持清空，先停止前台 Run 或对应 systemd 服务；仅退出 Run 不清空状态。

```sh
"$manager_bin" flush -n
"$manager_bin" flush
"$manager_bin" list
```

Flush 行为：

1. 获取修改锁，读取实际状态和管理记录。
2. 委托 brutalctl 清空全部 Brutal 规则及其普通路由清理范围，包含外部规则；它不是“仅删除 Go 规则”。
3. 对 Go 记录的策略 Route 属性，当前算法及关联 lock 状态仍匹配写入值时，移除 Go 实际添加 / 改写的属性；不匹配跳过，不恢复旧值。
4. 原本相同且未由 Go 添加的属性不清理；只添加过 lock 位时不删除原有算法。
5. Route / Table / 网关 / MTU 等其他字段保留；不删除 Peer Route 或整个 Table。
6. 局部清理失败继续其他条目，保存失败记录便于重试；全局故障停止后续修改。

Run 未停止时，后续核对会恢复配置中的规则。直接执行 brutalctl flush 不执行 Manager 的策略属性清理。

## 7. 管理记录异常恢复

| 状态 | 行为 |
|---|---|
| 首次无记录、无初始化标记 | 按首次安装处理，不自动清空未知状态 |
| 初始化标记存在，记录确认丢失 / 损坏 | 触发已确认的全量恢复重建 |
| 标记和记录都不可用 | 无法辨别历史，按首次安装处理；需要清空时显式 Flush |

异常重建在当前 JSON 全局校验通过后执行：记录阶段与配置版本 → Flush 全部 Brutal 规则 → 清理当前网络命名空间所有表中算法为 brutal 的属性和对应锁位 → 应用当前有效规则 → 保存新归属记录。

该流程是正常 Apply 不全量 Flush、保留外部规则的例外，会影响所有 Brutal 规则及跨表 brutal 属性；失败域名的旧规则不保证恢复。其他拥塞算法和路由字段保留。

断点恢复不得反复 Flush。进入重建阶段后，只处理未完成任务；旧清理重试前检查最新配置及归属，新规则已使用的 Route 取消旧清理。不要手工删除阶段记录来强制重跑。

## 8. 故障处置与结果判定

| 场景 | 处理 |
|---|---|
| 全局语法 / 顶层 / Settings / Resolver 定义错误 | 拒绝本次修改；Run 重载保留上一版 |
| 单条配置类型、CIDR、Rate 错误 | 跳过该条，其他有效规则继续 |
| DNS 查询故障 | 保护能识别的旧来源；其他规则继续 |
| DNS 成功整体无地址 | 删除该来源不再需要的旧状态，保留共享需求 |
| 非法新规则无法识别旧来源 | 仅保护可识别来源，其余旧受管规则仍可删除 |
| DNS 选择规则正则或引用错误 | 跳过选择规则，继续后续匹配，否则首个 Resolver |
| 新 Domain 部分操作失败 | 保留成功新地址和整组旧地址，修正 / 后续重试 |
| 顺序无法无损修复 | 保留现状，报告退化；正常 Apply 不强制 Flush |
| 操作中断或局部写入失败 | 回读实际状态，保存已完成项，修复问题后重试；不承诺回滚 |

退出码：

| Code | 含义 |
|---|---|
| 0 | 成功 / 无差异 |
| 1 | 运行时全局失败 |
| 2 | 全局配置或 CLI 参数错误 |
| 3 | Check / Diff 检测到普通差异 |
| 4 | 全局验证失败 |
| 5 | 全局权限错误 |
| 6 | 依赖 / 内核支持错误 |
| 7 | 局部失败、跳过、部分完成或暂缓重排 |

Check / Diff 优先级为全局错误 → 退化 7 → 普通差异 3 → 一致 0；单条验证失败纳入退化，不当作整次全局失败。Run 正常信号退出为 0，不能用它替代运行期间健康状态。Apply Dry Run 有普通差异时的最终退出码仍需在 CLI 实现前明确，不能自行假设等同 Check / Diff。

故障后先看规则来源和日志，修正配置 / DNS / Route，再 Check、Dry Run 和 Apply；需要保持清空则先停止 Run 再 Flush。不将“部分完成”当作“完全同步”。

## 9. 执行验收清单

- 输入单位、CIDR 默认值和 IPv4 优先数值排序正确。
- Dry Run 无系统或管理记录写入；读取诊断与文件日志边界明确。
- 普通路由只委托 brutalctl，策略 Route 修改保留无关属性。
- DNS 故障保护旧来源，成功无地址执行删除；共享 Prefix 不提前清理。
- 新规则及对应 Route 验证前不删除旧地址。
- 外部规则保留、外部策略操作删除冲突隔离，显式 Flush 范围明确。
- Run 各模块独立触发，非 DNS 事件不重复查 DNS。
- 全局失败与局部失败分离；恢复阶段重试不重复全量清空。
- 基于隔离 Linux 环境验证 brutalctl v2.0.1 的插入方向、速率范围、路由副作用与 Netlink 锁位保留，再进入编译和发版阶段。
