# TCP Brutal Config Go 项目完整开发计划

## 已确认实施约束（更新于 2026-10-12）

本节为逐项确认后的统一约束；旧示例中 Desired / Current 相等指受管范围，无外部规则且无退化状态时才要求整个规则列表一致。

1. Resolver 按唯一 name 引用，重名拒绝。
2. DNS 匹配支持 full / domain / keyword / regexp，无前缀为 full；首次命中优先。domain 匹配自身和任意层级子域名；匹配目标忽略大小写和末尾点。不另设星号格式，不引入外部域名列表。
3. fallback 默认 false；显式开启后按 Resolver 数组顺序回退，跳过已尝试项。NXDOMAIN / NODATA 不回退。
4. 成功整体无地址时清理该来源旧规则；查询故障保护旧规则。单族有地址而另一族 NODATA 可正常应用。
5. DNS 单次 timeout 默认 5s；Route / Brutal 单次 operation_timeout 默认 10s，均放入对应 Settings 模块，可配置。
6. Rate 默认单位 Mbps，自定义单位仅限 bit 速率单位，最终精确转换为正整数 Mbps 后传给 brutalctl；不静默舍入。内部统一 RateMbps，详见第八节。
7. 业务规则 lock 默认 true、noroute 默认 false；指定 table 或 interface 时控制接口强制 noroute，Route 拥塞算法锁与规则 lock 联动：true 为 congctl lock brutal，false 为 congctl brutal 并清除 RTAX_CC_ALGO 对应锁位；其他 metrics 锁位保留。
8. 空 rules 数组属于全局配置错误，Run 保留最后有效配置；主动清空用 flush。
9. 允许不同 Prefix 的重叠网段，依赖最长前缀匹配；相同 Prefix 参数冲突隔离所有涉及规则。
10. 允许覆盖已有 Route 拥塞算法；记录 Go 实际写入属性，清理匹配值时移除，不恢复旧算法，保留其他属性。仅新增 lock 位而算法原本相同时，记录和清理仅新增位；匹配清理不能删除原有算法。
11. 普通 Apply 保留外部 Brutal 规则。同 Prefix 参数不同则跳过配置规则，参数相同可复用但不认领；无关 Prefix 保留。显式 flush 清空整个 Brutal Ruleset。
12. 配置删除规则或 DNS 成功整体无地址时，同步清理不再需要的 Go Route 属性；共享来源仍有有效或受保护规则需要时保留，不删除 Peer Route。
13. Run 的 DNS 查询只由启动、DNS query_interval 到期、相关业务域名或 Resolver 配置变化触发；Route / Brutal / 配置兜底事件不无条件触发 DNS。使用本进程最近成功结果，重启不使用跨进程 DNS 缓存。
14. 可用性优先：受管规则顺序参与 Diff；无法无损重排时保留运行状态、报告差异，不 Flush 重建。保留的外部规则不参与配置顺序一致性要求。
15. 局部失败保护对应旧规则并继续其他独立规则；全局预检失败禁止整次修改。运行中全局失败停止后续修改，已完成修改如实报告，不保证回滚。普通部分完成返回 7，全局错误返回对应错误码。Check / Diff 的结果优先级按第 26 项执行。
16. 首版支持与测试基线为 TCP Brutal v2.0.1；其他版本需通过兼容性检查。实现核对需验证插入方向、Rate 范围及精度、Route metrics / lock 位解析与无关属性保留，不将调查中的订阅成功当作事件修改实测通过。

17. lock 同时控制规则和 Route 的拥塞算法锁；true 锁定，false 清除仅对应 RTAX_CC_ALGO 的锁位，其他锁位保留。
18. 相同 Prefix、全部参数相同的规则 Stable Dedup 后保留全部来源；所有来源均不再需要且无失败来源保护时，才清理共享规则与属性。
19. 非法规则无法关联旧来源时，只保护能识别的失败来源，其余旧受管规则可删除；不因为一个来源无法识别而暂停全部删除。
20. 管理记录损坏或确认丢失时，执行异常恢复重建：全量 Flush Brutal 规则，移除当前网络命名空间所有表中当前算法为 brutal 的属性和对应锁位，再按当前 JSON 的有效规则重建。不恢复旧算法，不修改其他算法、路由字段或锁位。这是普通 Apply 不 Flush、保留外部规则的明确例外。首次安装尚无记录不触发该恢复。
21. Flush 单条 Route 清理失败继续其他条目，汇总错误并保留失败记录；全局故障停止后续修改。
22. 删除日志级别 none；只允许 debug / error / warning / default，关闭文件日志用 enabled=false。
23. DNS 选择规则无效（包括正则错误、引用不存在的 Resolver）时跳过该条并报告退化；后续规则继续匹配，未命中用首个 Resolver。Resolver 定义非法或重名仍属全局错误。
24. Run 订阅 Netlink Link 事件，接口删除、重建或改名触发受影响 Route 重定位；局部失败保护旧状态，不触发无关 DNS 查询。
25. 管理记录保存完整已成功应用状态：Prefix、Rate、Lock、Route 策略及所有来源；重启 DNS 故障时据此保护旧状态，不代替新查询。
26. Check / Diff 退出结果按全局错误、退化 7、普通差异 3、一致 0 优先；已确认无法无损处理的重排为退化，普通待更新顺序差异为差异。

27. Domain 多地址更新部分成功时保留成功新地址与整组旧地址，只有全部新地址及 Route 操作验证成功才清理过期旧地址；成功整体无地址仍按第 4 项清理。
28. 配置分层解析：JSON 语法及顶层结构非法拒绝整份；业务 rules / dns.rules 先保存为有序 json.RawMessage，再逐条强类型解码与校验，类型错误隔离到对应条目。
29. 恢复日志持久化阶段、配置内容版本与操作归属；重建阶段不重复 Flush。旧清理重试前核对当前有效配置和新写入归属，已被新规则使用的 Route 取消旧清理，不执行过时计划。
30. 每次 Route 修改或匹配清理前重读最新属性和身份，仅修改拥塞算法及对应 lock 位，保留其他最新属性；写后验证。身份变化或外部持续修改时局部暂缓，不宣称跨进程原子性。
31. 正文、接口、流程与测试统一到以上行为：受管范围 Diff、Domain 展开追踪来源并按最终数值规则排序、单条增删改、DNS 在系统修改锁外执行，锁内重新读取状态和生成计划；显式 Flush 与记录异常恢复为全量清空例外。

32. Desired 最终顺序：IPv4 在前、IPv6 在后；各族按规范化网络地址数值升序，相同网络地址按 CIDR 长度升序。业务 JSON 顺序不再决定最终顺序；Domain 地址全局参与排序、不要求连续分组。冲突检测与保留全部来源的 Stable Dedup 在排序前完成。Current 保留内核原顺序，禁止排序掩盖差异；可用性优先及无损重排约束仍保留。

33. 继续调用官方 brutalctl；将 Rate 规范化为整数 Mbps，委托普通路由管理，仅对策略操作检查路由副作用。普通 Apply 的 add / noroute / del 若会删除外部 proto 233 路由则跳过对应规则；受管路由副作用纳入计划并回读验证。运行依赖包含 brutalctl 及其调用的 iproute2。
34. 初始化标记不存在且记录不可用时按首次安装处理，保留未知归属状态，不自动清空；标记仍在且记录损坏 / 丢失时才自动异常重建。无标记需要清空时使用显式 Flush。
35. Run 重载先准备新 Logger，成功后连同配置快照切换；失败拒绝新版本、保留旧配置与 Logger，并写 stderr。enabled=false 不打开文件。

36. 项目定位为 TCP Brutal Manager：Rate 默认 Mbps，仅支持 bit 速率单位并精确换算为整数 Mbps；未指定 interface / table 的普通路由交由 brutalctl 管理。Go 主要负责统一管理、更新及维护规则，以及显式策略 Route 属性。此项取代此前内部 bit/s 与普通 Route 接管/修复建议。

验收增加上述匹配类型、默认值、整体空结果删除、部分查询故障保护、外部规则复用不认领、原有算法覆盖与匹配清理、共享 Route 归属，以及非 DNS 事件不查询 DNS 的测试。

---

## 一、项目目标

使用 **Go** 开发 **TCP Brutal Manager**：以官方 `brutalctl` 为控制接口，集中进行配置管理、DNS 解析、规则更新、状态维护和验证。未指定接口或路由表的普通路由交由官方工具管理；Go 仅维护显式策略路由的拥塞算法属性。

项目仓库：

```text
XHQ6666/tcp-brutal-config-go
```

核心流程：

```text
JSON 配置
   ↓
解析基础配置
   ↓
初始化日志
   ↓
完整配置校验
   ↓
DNS 实时解析
   ↓
展开并去重后按地址族、网络地址、CIDR 长度生成 Desired State
   ↓
读取 TCP Brutal Current State
   ↓
读取 Linux Route Current State
   ↓
差异比较
   ↓
生成 Apply Plan
   ↓
仅在存在差异时修改
   ↓
Post-Apply Verification
   ↓
输出日志
```

项目严格遵循：

**计划 → 实现 → 核对 → 编译 → 发版**

每一步开始前确认。

---

## 二、设计原则

Go 正式版从一开始必须遵循以下原则：

- 配置驱动
- 强类型配置结构
- 有序规则模型
- DNS 每次执行实时解析
- 默认不持久化缓存 DNS 记录
- Domain 展开追踪全部来源
- Stable Dedup
- Prefix 冲突检测
- 顺序感知 Diff
- Desired State / Current State 分离
- Route Difference 与 Brutal Difference 分离
- 修改前完整验证
- Route-Only Repair
- Dry Run
- Post-Apply Verification
- 并发安全
- 结构化日志
- 明确 Exit Code
- 尽量减少 Shell 工具依赖
- 所有系统修改必须可追踪

最终目标：

```text
JSON Config 的有效规则
→
有序 Desired State
→
实际受管 Brutal 状态；局部失败与暂缓顺序差异明确报告
```

正常完整同步的受管顺序目标为（外部规则不计入，无法无损重排可暂缓）：

```text
IPv4 优先、网络地址及 CIDR 长度升序
=
展开、去重并排序后的 Desired 顺序
=
/proc/net/tcp_brutal/rules 中受管规则顺序
=
brutalctl list 中受管规则顺序
```

---

## 三、项目结构

建议项目结构：

```text
cmd/
└── tcp-brutal-config/
    └── main.go

internal/
├── config/
│   ├── config.go
│   └── validate.go
├── dns/
│   ├── resolver.go
│   ├── system.go
│   ├── doh.go
│   ├── dot.go
│   ├── doq.go
│   ├── fallback.go
│   └── bootstrap.go
├── rules/
│   ├── normalize.go
│   ├── expand.go
│   ├── dedup.go
│   └── conflict.go
├── brutal/
│   ├── current.go
│   ├── apply.go
│   └── verify.go
├── route/
│   ├── current.go
│   ├── validate.go
│   ├── repair.go
│   └── netlink.go
├── state/
│   ├── desired.go
│   ├── current.go
│   └── diff.go
├── sync/
│   ├── plan.go
│   └── apply.go
├── log/
│   ├── logger.go
│   └── file.go
├── lock/
│   └── lock.go
└── cli/
    └── cli.go

examples/
└── config.json

systemd/
└── tcp-brutal-config.service

tests/

README.md
LICENSE
go.mod
go.sum
```

模块边界必须清晰，避免把 DNS、配置解析、Linux Route 和 Apply 逻辑全部堆入 `main.go`。

---

## 四、JSON 配置

完整配置示例：

```json
{
  "settings": {
    "dns": {
      "query_interval": "60s",
      "timeout": "5s"
    }
  },
  "log": {
    "enabled": true,
    "level": "default",
    "path": "/var/log/tcp-brutal-config.log"
  },
  "dns": {
    "resolvers": [
      {
        "name": "primary",
        "type": "DoQ",
        "address": "dns.example.com",
        "port": 853
      },
      {
        "name": "secondary",
        "type": "DoH",
        "address": "dns.example.com",
        "path": "/dns-query",
        "port": 443
      },
      {
        "name": "system",
        "type": "System"
      }
    ],
    "rules": [
      {
        "domain": ["example.com", "ipv4.example.com"],
        "resolver": "primary",
        "fallback": true
      }
    ]
  },
  "rules": [
    {
      "ip": "192.0.2.1",
      "rate": "200Mbps"
    },
    {
      "domain": "ipv4.example.com",
      "rate": "350Mbps"
    },
    {
      "ip": "2001:db8:1234::",
      "cidr": 48,
      "rate": "350Mbps"
    },
    {
      "ip": "198.51.100.1",
      "rate": "200Mbps",
      "table": 52,
      "interface": "tailscale0"
    }
  ]
}
```

配置分为：

```text
settings
log
dns
rules
```

---

### 分层配置解析

顶层字段强类型处理；`rules` / `dns.rules` 先按有序 `[]json.RawMessage` 接收，再逐条解码为强类型配置。局部字段类型错误保留索引与错误原因，跳过该条。正常完整配置模型表示有效条目，不以整份一次性强类型解码破坏局部容错。

---

## 五、Go 配置模型

建议使用强类型结构：

```go
type Config struct {
    Settings SettingsConfig `json:"settings"`
    Log   LogConfig   `json:"log"`
    DNS   *DNSConfig  `json:"dns,omitempty"`
    Rules []Rule      `json:"rules"`
}
```

规则必须使用 Slice：

```go
[]Rule
```

禁止使用：

```go
map[string]Rule
```

作为规则主数据结构。

原因：

**业务规则仍按 Slice 解析以保留来源与稳定去重过程；最终 Desired 顺序由地址族、规范化网络地址与 CIDR 长度决定。DNS 选择规则仍按 JSON 首次命中顺序处理。**

---

## 六、Rule 配置

规则支持：

```text
ip
domain
rate
cidr
lock
noroute
table
interface
```

基本约束：

```text
ip 与 domain 互斥
```

以下配置非法：

```json
{
  "ip": "192.0.2.1",
  "domain": "example.com",
  "rate": "200Mbps"
}
```

同样不能两者都不存在。

逐条配置校验时拒绝该规则并记录错误，不阻断其他有效规则。

---

## 七、IP / Domain 与 CIDR 长度（已确认）

配置字段统一为 `cidr`，移除旧 `prefix` 字段；旧字段视为非法规则配置，不静默忽略。内部仍使用 `net/netip` 和 `netip.Prefix` 表示规范化网段，不将内部 Prefix 类型重命名。

### IP Rule

`ip` 填纯 IPv4 / IPv6 地址，网段长度通过可选整数 `cidr` 指定；为保持长度来源唯一，`ip` 中不接受带 `/长度` 的 CIDR 字符串。

```json
{
  "ip": "192.0.2.1",
  "cidr": 24,
  "rate": "200Mbps"
}
```

规范化为 `192.0.2.0/24`。IPv6 示例：

```json
{
  "ip": "2001:db8:1234:5678::1",
  "cidr": 48,
  "rate": "350Mbps"
}
```

规范化为 `2001:db8:1234::/48`。

IP Rule 的 `cidr` 仅接受整数；省略时 IPv4 默认 32、IPv6 默认 128。显式 `0` 为合法网段长度，不能被当作省略。

### Domain Rule

`cidr` 支持整数，或分别指定地址族的对象：

```json
{
  "domain": "example.com",
  "cidr": {"v4": 24, "v6": 64},
  "rate": "350Mbps"
}
```

- 不配置 `cidr`：A 地址使用 `/32`，AAAA 地址使用 `/128`。
- 数字形式：对实际返回的每个地址族使用同一个长度。
- 对象形式：`v4` 对应 IPv4，`v6` 对应 IPv6；缺少任意一个字段时，该地址族使用最大长度；空对象等同于未配置。
- 例如 `{"v6": 64}` 表示 IPv4 `/32`、IPv6 `/64`。
- 各地址规范化后仍在原 Domain 位置展开，再进行 Stable Dedup 与冲突检测。

### 校验与失败隔离

IPv4 有效范围为 0–32，IPv6 为 0–128；只接受 JSON 整数，拒绝负数、小数、字符串、布尔值、null、数组及对象未知字段。IP Rule 不接受对象形式。

Domain 的整数形式必须位于 0–128；解析结果中如果包含 IPv4 而长度超过 32，该 Domain Rule 整体跳过并保护旧状态，其他独立规则继续。若只返回 IPv6，则允许 33–128。对象字段分别按对应地址族范围预先校验。

使用 `netip.PrefixFrom(addr, bits).Masked()` 生成 canonical representation；禁止通过字符串截取实现 IPv6 网段。

填写 `table` / `interface` 时，Route 定位必须精确匹配规范化后的完整 Prefix，不能用主机路由替代网段路由。

测试覆盖 IPv4 / IPv6 默认长度、显式 0、主机位清零、Domain 数字与对象形式、单地址族字段缺省、空对象、双栈数字越界、非法字段类型、旧 prefix 拒绝及局部失败隔离。

---

## 八、Rate（Manager 输入与 Mbps 转换，已确认）

`rate` 接受 JSON 数字或字符串。无单位数字默认 Mbps；自定义单位仅支持 bit 速率单位 `bps`、`Kbps`、`Mbps`、`Gbps`，十进制换算，不支持 Byte 单位（如 MBps / MB/s）。

```json
{"ip": "192.0.2.1", "rate": 200}
```

数字字符串 `"200"` 同样表示 200 Mbps；`"200Mbps"` 和 `"1Gbps"` 分别转换为 200、1000。

最终统一转换为正整数 Mbps，并将整数的十进制字符串传给 `brutalctl`。内部 Rate 固定为 `RateMbps uint64`，Current 的内核 byte/s 与目标 Mbps 使用精确十进制关系比较。

不静默舍入：转换后非整数 Mbps 的值拒绝该规则。例如 `"2.5Gbps"` 合法（2500 Mbps），`"2.5Mbps"` 或 `"500Kbps"` 非法；正数、范围和整数校验在任何操作前完成。只支持目标版本可接受且已经验证的范围；拒绝零、负数、NaN / Inf、溢出及非法单位。

本项目负责输入单位规范化及管理，不复刻 brutalctl 内部 Rate 转换逻辑；操作后回读验证实际速率，差异按现有失败模型报告。

测试覆盖无单位数字 / 数字字符串、各 bit 单位、转换后整数判断、Byte 单位拒绝、溢出及 Current 速率比较。

---

## 九、Lock

配置：

```json
{
  "lock": true
}
```

内部统一使用：

```go
bool
```

读取 `/proc/net/tcp_brutal/rules` 时可能遇到：

```text
1
0
true
false
yes
no
on
off
```

统一规范化：

```text
1 / true / yes / on
→ true

0 / false / no / off
→ false
```

解决此前出现过的：

```text
1 != true
```

导致的错误 Diff 和：

```text
post-apply verification failed
```

---

## 十、日志配置

配置：

```json
{
  "log": {
    "enabled": true,
    "level": "default",
    "path": "/var/log/tcp-brutal-config.log"
  }
}
```

支持字段：

```text
enabled
level
path
```

默认：

```text
enabled = true
level   = default
path    = /var/log/tcp-brutal-config.log
```

---

## 十一、日志级别

只允许：

```text
debug
error
warning
default
```

### debug

输出完整调试信息，包括：

- 配置解析
- DNS 查询
- DNS retry
- DNS fallback
- Prefix 规范化
- Domain 展开
- Stable Dedup
- 冲突检测
- Current State
- Diff
- Apply Plan
- Brutal 操作
- Route 操作
- Dry Run
- Verification
- Lock
- Route-Only Repair

### error

仅记录 ERROR 和 Fatal Error。

### warning

记录 WARNING、ERROR、Fatal Error。

### default

记录正常运行摘要，包括启动、配置加载、DNS 查询摘要、Desired State 生成、Diff 摘要、Apply、Route Repair、Verification 和完成，但不记录逐命令 Debug 信息。

---

## 十二、日志格式

统一格式：

```text
TIMESTAMP [LEVEL] [COMPONENT] message fields...
```

例如：

```text
2026-10-06T01:30:00+08:00 [INFO] [dns] resolved domain=example.com addresses=2
2026-10-06T01:30:01+08:00 [WARNING] [route] route attribute missing prefix=198.51.100.1/32
2026-10-06T01:30:02+08:00 [ERROR] [apply] operation failed error="verification failed"
```

组件使用稳定集合：

```text
config
log
dns
rules
state
diff
apply
route
lock
verify
cli
```

禁止无意义标签堆叠。

---

## 十三、日志路径

支持绝对路径和相对路径。

Go 版相对路径统一相对于：

```text
配置文件所在目录
```

解析，不能依赖当前 shell working directory。

日志初始化后记录最终绝对路径。

---

## 十四、日志文件行为

日志文件使用追加模式：

```text
append
```

禁止每次执行清空。

文件权限遵循最小权限原则，Go 写入必须支持并发安全。

第一版不要求内置复杂日志轮转，可以交由 `logrotate` 或 `systemd-journald` 管理。

Run 日志重载先验证新配置并准备新 Logger，再在串行协调器边界连同配置一起切换，释放旧 Logger；新文件创建或写入检查失败时拒绝新配置，保留旧 Logger 与旧配置并输出 stderr。enabled=false 的新版本不打开文件。

---

## 十五、日志初始化失败

如果 `enabled=false`，日志文件完全不参与执行；必要配置、权限及致命错误仍输出 stderr。

如果 `enabled=true` 且日志文件无法创建或写入，默认终止 apply，避免在无法记录系统修改时继续改变系统状态。

同时必须输出 stderr，并保证 Brutal Ruleset 与 Linux Route 不变。

---

## 十六、DNS 配置

DNS 由 Go 正式版原生实现。

```json
{
  "dns": {
    "resolvers": [
      {
        "name": "primary",
        "type": "DoQ",
        "address": "dns.example.com",
        "port": 853
      },
      {
        "name": "secondary",
        "type": "DoH",
        "address": "dns.example.com",
        "path": "/dns-query",
        "port": 443
      },
      {
        "name": "system",
        "type": "System"
      }
    ],
    "rules": [
      {
        "domain": ["example.com", "ipv4.example.com"],
        "resolver": "primary",
        "fallback": true
      }
    ]
  }
}
```

支持：

```text
DoH
DoT
DoQ
System
```

配置结构：

- `dns.resolvers[]` 定义 Resolver 的 `name`、`type`、`address`、`port`，以及 DoH 专用的 `path`；Resolver 定义中不再包含 `fallback`。
- `dns.rules[]` 定义 `domain`、`resolver`、`fallback`。
- `domain` 支持字符串或非空字符串数组；例如 `"example.com"` 与 `["example.com", "another.example.com"]`。内部统一规范化为有序域名列表。
- 数组中的每个域名共用该 DNS Rule 的 Resolver 与 Fallback 设置，但独立解析、独立报告失败；其中一个失败不跳过同数组的其他域名。
- 该数组用于 DNS Resolver 选择，不改变业务 `rules[]` 的展开顺序。
- 未配置 `dns` 时，使用 System Resolver；已配置 `dns.resolvers` 时，未匹配 DNS Rule 的域名默认使用 `resolvers[]` 首项，不自动选择 System。
- 已配置 `dns` 但未提供 Resolver 或 Resolver 列表为空时，使用 System Resolver 作为隐式首项；规则若引用不存在的命名 Resolver，跳过该 DNS 选择规则并报告退化。
- `fallback` 位于 `dns.rules[]`：指定 DNS 解析器故障时是否允许回退其他 Resolver；有效的 NXDOMAIN / NODATA 不触发回退。
- `resolver` 引用唯一的 Resolver `name`；重复名称属于全局配置错误；选择规则引用不存在的名称属于该条局部错误。
- DNS 选择规则支持 `full:` 精确匹配、`domain:` 匹配自身及任意层级子域名、`keyword:` 子串匹配、`regexp:` 正则匹配；无前缀按 `full:` 处理。不另设 `*.` 通配格式，不支持 geosite / ext / dotless。
- 按 DNS Rule 数组顺序首次命中；同规则的域名数组为 OR 关系。匹配目标统一转小写并去除末尾点；full / domain / keyword 的配置值同样规范化，regexp 表达式保留原文，作用于规范化后的目标。
- `fallback` 默认 false。显式 true 时，指定 Resolver 耗尽最多三次尝试后按 `resolvers[]` 顺序尝试其他项，跳过已尝试项；未匹配 DNS Rule 时用首项且不自动回退。
- 匹配格式仅用于 `dns.rules[].domain` 选择 Resolver；业务 `rules[].domain` 必须为可查询的实际域名。
- 成功返回 NXDOMAIN 或 A / AAAA 合计无地址时，清理该来源不再需要的旧受管规则及 Route 属性；单族 NODATA 而另一族有地址时正常应用。任一族最终查询故障时保护旧规则，不应用半份结果。
- 拒绝空数组、空域名或非字符串元素。

测试覆盖单字符串、单元素数组、多域名数组、非法数组，以及数组中一个域名失败不影响其他域名。

---

## 十七、DNS 默认端口

```text
DoH = 443
DoT = 853
DoQ = 853
```

允许显式覆盖。

### DoH Address 与 Path（已确认）

DoH 的 `address` 填写 DNS 服务域名，不填写完整 URL；请求协议固定为 HTTPS，端口使用 `port`（默认 443）。

```json
{
  "name": "secondary",
  "type": "DoH",
  "address": "dns.example.com",
  "path": "/dns-query",
  "port": 443
}
```

`path` 未配置时默认 `/dns-query`。显式配置的 `path` 必须是以 `/` 开头的非空请求路径，不能填完整 URL。请求 Endpoint 使用 URL 构造器从 HTTPS、Address、Port、Path 构建，不能将域名当成已含协议的 URL 解析。

测试覆盖显式 Path、缺省 Path、自定义端口，以及 Address 填完整 URL 或 Path 非法时拒绝配置。

`System` 不需要 `address` 和 `port`。

非法 DNS 配置必须在进入规则解析前拒绝。

---

## 十八、DNS 查询原则

**默认不持久化缓存 DNS 记录。**

每次执行：

```text
check
diff
apply
dry-run
```

都必须重新查询 Domain。

Run 保留本进程最近成功解析结果用于非 DNS 事件的状态修复；仅启动、DNS 定时器到期或相关 Domain / Resolver 配置变化触发新查询。进程重启必须重新解析；持久化来源记录只用于归属与失败保护，不能替代新 DNS 查询。一次性命令不复用跨次结果。

---

## 十九、DNS 查询类型

对于 Domain Rule：

```json
{
  "domain": "example.com",
  "rate": "350Mbps"
}
```

查询：

```text
A
AAAA
```

查询结果生成对应 IPv4 `/32`、IPv6 `/128`，或者根据 Rule 中的 `cidr`（数字或 Domain 专用的 v4 / v6 对象）进行规范化；缺省地址族长度使用最大值。

---

## 二十、DNS Retry

Resolver Failure：

```text
Attempt 1
   ↓ 3 秒
Attempt 2
   ↓ 3 秒
Attempt 3
   ↓
Failure
```

最多 3 次，间隔 3 秒。

必须支持：

```go
context.Context
```

允许 CLI Cancel、systemd stop 和 timeout 正确终止等待。

---

## 二十一、DNS Fallback

先使用匹配的 `dns.rules[]` 所指定的 Resolver；允许 Fallback 时按 `dns.resolvers[]` 顺序尝试其他 Resolver（按唯一名称引用，跳过已尝试项）。

只有 Resolver Failure 才进入 Fallback，例如：

- Timeout
- Connection Failure
- TLS Failure
- QUIC Failure
- Malformed Response
- SERVFAIL

明确的：

```text
NXDOMAIN
NODATA
```

属于有效 DNS 响应，不能因为 `fallback=true` 就继续换 Resolver 查询。

---

## 二十二、DNS Bootstrap

DoH / DoT / DoQ Endpoint 如果使用域名，必须解决 Bootstrap 问题。

第一版允许：

```text
System Resolver
↓
Bootstrap DoH / DoT / DoQ
```

Bootstrap 默认同样不持久化缓存，每次新的 CLI 执行重新获取 Endpoint 地址；Run 每轮重新解析 Bootstrap 地址。

未来可以增加显式 Bootstrap IP 配置。

---

## 二十三、Domain 展开与来源

按业务规则 Slice 遍历并解析，每个 Domain 返回的地址分别规范化，保存原始配置来源。Domain 展开仅是候选生成步骤，不以 JSON 中位置决定最终 Kernel 顺序。

---

## 二十四、Domain 多地址与共享规则

Domain 可生成多个 IPv4 / IPv6 Prefix，所有候选与 IP Rule 一起冲突检测、Stable Dedup 并合并全部来源，之后执行全局数值排序。不要求最终 Domain Group 连续。

Domain 更新失败隔离仍以来源为单位，不能因排序打散而丢失整组旧地址保护语义。

---

## 二十五、Desired 排序（已确认）

固定比较键为：

```text
地址族（IPv4 < IPv6）
→ 规范化网络地址数值升序
→ CIDR 长度升序
```

使用 `netip.Addr.Compare` 与 Prefix Bits，禁止以字符串字典顺序比较地址。先执行 Masked，再冲突检测和 Stable Dedup，最后排序；不能以排序隐藏相同 Prefix 参数冲突。

示例：

```text
192.0.2.0/24
192.0.2.0/32
192.0.2.2/32
192.0.2.10/32
2001:db8::/32
2001:db8::1/128
```

A / AAAA 返回顺序只影响候选与来源记录，不决定最终顺序。仅 Desired 排序，Current 按 proc 原始顺序读取与 Diff；外部保留规则不计入受管顺序比较。无法无损重排时暂缓并报告退化，不 Flush 强行重建。

---

## 二十六、Desired Rule 模型

建议：

```go
type DesiredRule struct {
    Prefix    netip.Prefix
    RateMbps  uint64
    Lock      bool
    NoRoute   bool
    Table     *int
    Interface string

    Sources []RuleSource // 去重后保留全部来源与 Domain 更新组
}

type RuleSource struct {
    ID          string // 配置路径、类型、源值、重复序号
    Type        string
    Index       int    // 诊断位置，非身份
    Value       string
    UpdateGroup string
}
```

来源字段用于 Diff、Logging、Conflict Error、Domain 来源追踪和 Debug。

---

## 二十七、Stable Dedup

禁止通过任何排序实现去重。

Go 版本必须实现 Stable Deduplication：

```text
A
B
A
C
B
```

得到：

```text
A
B
C
```

Stable Dedup 阶段保留首次候选并合并全部来源，随后最终 Desired 按数值规则排序；去重测试验证的是排序前的中间结果。

实现可以使用：

```go
map[RuleKey]int
```

作为是否见过该 Rule 的索引表，但输出仍必须写入 `[]DesiredRule`，禁止遍历 Map 生成最终 Rule 顺序。

---

## 二十八、冲突检测

完全相同的：

```text
prefix + rate + lock + relevant options
```

允许 Stable Dedup。

同一个 Prefix 对应不同参数时必须报错。

禁止：

- Last Write Wins
- First Write Wins
- 静默覆盖
- 排序后隐藏冲突

---

## 二十九、Current Brutal State

Current State 来源：

```text
/proc/net/tcp_brutal/rules
```

Go 直接读取文件：

```text
os.Open()
↓
bufio.Scanner
↓
逐行解析
↓
字段规范化
↓
[]CurrentRule
```

必须保持 `/proc/net/tcp_brutal/rules` 原始顺序，禁止重新排序。

---

## 三十、Current Rule 模型

建议：

```go
type CurrentRule struct {
    Prefix  netip.Prefix
    RateBytesPerSecond uint64 // 原样保存内核数值，不取整为 Mbps
    Gain    int
    Lock    bool
    ID      int
    Members int
    Sent    uint64
}
```

Current Rate 保留内核 byte/s 原值；Desired 整数 Mbps 转换为目标 byte/s 后精确比较，不能截断 Current 到整数 Mbps 隐藏差异。

真正参与 Desired Diff 的核心字段：

```text
Prefix
Rate
Lock
Order
```

其他动态字段主要用于 Status、Debug 和 Diagnostics。

---

## 三十一、顺序属于 State

以下状态：

```text
Desired:
A
B
C

Current:
A
C
B
```

必须判断为 `DIFFERENT`。

Diff 必须识别：

```text
Content Difference
Parameter Difference
Order Difference
Added Rule
Removed Rule
DNS-Derived Difference
```

---

## 三十二、Apply 顺序

必须根据当前 TCP Brutal 的实际插入行为决定 Apply 方向。

如果：

```text
add A
add B
add C
```

内核最终为：

```text
C
B
A
```

则实际 Apply 必须逆序。

不能凭假设编码，必须以当前 TCP Brutal 版本实测确认。

正常完整同步目标：

```text
Desired State Order
=
Kernel 中受管规则顺序
```

外部规则不参与受管顺序比较；无法无损调整时报告退化并保留运行状态，不 Flush 强行重排。

---

## 三十三、Brutal 管理

第一版允许继续调用官方 `brutalctl` 作为 TCP Brutal 控制接口。

但命令执行必须通过 Go `os/exec` 统一封装，例如：

```go
type BrutalController interface {
    Flush(ctx context.Context) error // 仅显式 Flush / 异常恢复
    Add(ctx context.Context, rule DesiredRule) error
    Update(ctx context.Context, rule DesiredRule) error // 已有 Prefix 原位更新
    Delete(ctx context.Context, prefix netip.Prefix) error
}
```

不得在业务层到处直接调用 `exec.Command(...)`。

### Manager 与 Brutalctl 职责（已确认）

- 项目定位为 TCP Brutal Manager，负责配置、DNS、单位转换、规则归属、增量更新、持续维护与验证；不重新实现官方控制工具的普通路由管理。
- 规则没有 `interface` / `table` 时，Add / Update / Delete / Flush 的普通路由行为交给 brutalctl；显式 noroute 同样交由其处理。Go 不替普通路由重新做选路、建路或修复算法，也不按自己的 Route Plan 覆盖官方结果。
- 普通路由状态可读取用于诊断；是否添加路由及失败结果由官方工具处理，并按规则容错策略记录；规则和路由可能部分成功，应回读实际状态。
- 指定 `interface` 或 `table` 时，brutalctl 使用 noroute 创建 / 更新 Group，Go 定位已有 Route 并按 lock 配置维护拥塞算法属性。保留此前策略路由安全校验、属性归属和匹配清理规则。
- 仅策略 Route 操作纳入 Go 的 Repair / Cleanup；对 brutalctl 在策略规则 noroute / del 时潜在的 proto 233 删除副作用仍进行外部冲突校验，不自行接管普通路由生命周期。
- Rate 预先转为正整数 Mbps，直接传十进制整数参数；不复刻官方浮点实现。回读内核 Rate 验证转换结果，支持范围通过 v2.0.1 兼容测试确定。
- 运行依赖为 brutalctl 及其使用的 iproute2；官方隐式行为在日志 / Dry Run 标注为委托操作，不伪造 Go 未执行的 Netlink 修改计划。
- 显式 Flush 先委托 brutalctl 清空规则及其管理的普通路由，再按 Go 记录清理策略 Route 属性；管理记录异常恢复的跨表 brutal 属性清理保持已确认例外。

DomainUpdateGroup 至少保存全部来源、目标 Prefix、旧保护 Prefix、逐项操作及验证结果；全部新项成功前不能释放旧保护集合，DNS 成功整体空结果为明确清理例外。


---

## 三十四、普通 Route（委托官方管理）

没有指定 `interface` 和 `table` 的规则，由 brutalctl 自行处理普通路由；Go Manager 不复制其选路、创建、删除和修复流程。

`noroute` 按配置传递。没有参数时默认允许 brutalctl 管理路由；lock 默认 true，显式 false 按官方 nolock 行为传递。

运行结果按官方命令和 proc 回读判断，错误隔离到规则；普通路由诊断与 Go 管理的策略 Route Diff 必须区分，不为普通路由另外生成 Netlink Repair / Cleanup。

---

## 三十五、策略路由

支持：

```json
{
  "ip": "198.51.100.1",
  "rate": "200Mbps",
  "table": 52,
  "interface": "tailscale0"
}
```

这种 Rule 表示：

1. 创建 TCP Brutal Group；
2. 不让 Brutal 创建错误的 Main Table Route；
3. 将 `congctl lock brutal` 应用到实际 Route。

目标：

```text
table 52
198.51.100.1/32 dev tailscale0 congctl lock brutal
```

对应 Brutal Group 使用 `noroute`。

---

### 策略路由字段单独填写与自动定位（已确认）

`table` 与 `interface` 允许单独填写，不要求成对出现。

| 配置方式 | 定位与校验行为 |
|---|---|
| 仅填写 `interface` | 枚举当前网络命名空间内的路由表，查找目标 Prefix 精确匹配且出口接口匹配的已有路由；唯一匹配后读取实际 Table |
| 仅填写 `table` | 在指定 Table 中查找目标 Prefix 精确匹配的已有路由；唯一匹配后读取实际 Interface |
| 同时填写两者 | 在指定 Table 中按目标 Prefix 精确匹配查找已有路由，并验证出口 Interface |
| 两者都不填写 | 按普通 Route / `noroute` 配置处理，不进行策略路由自动定位 |

例如用户只知道接口名：

```json
{
  "ip": "100.100.100.100",
  "rate": "200Mbps",
  "interface": "tailscale0"
}
```

必须从已有路由中定位实际路由表，不能假设表号为 52，也不能使用默认出口 ens3 代替指定接口。

定位规则：

- 使用规范化后的目标 Prefix 精确匹配，不使用覆盖该目标的网段路由或默认路由作为替代。
- 未找到、找到多个候选或出口无法唯一确定时，直接报错；禁止猜测、静默选择或擅自迁移 Route。
- Domain 展开后，对每个规范化 Prefix 分别执行定位与校验。
- 所有规则的 Route 定位与安全校验在生成 Apply Plan 前完成；局部失败仅保护该规则旧状态，其余独立有效规则继续同步。
- 只要填写了 `table` 或 `interface` 任意一项，创建 Brutal Group 时必须使用 `noroute`，禁止创建错误的 Main Table Route。
- 保留定位到的已有 Route 属性，仅增加或恢复 `congctl lock brutal`；禁止凭空创建 Peer Route，不自动添加或修改 RPDB / `ip rule`。
- 自动定位出的 Table / Interface 纳入 Route Desired State、Diff、Apply Plan、日志和 Post-Apply Verification。
- Route-Only Difference 仍然只修复 Route，不 Flush Brutal Ruleset。

验证必须覆盖：仅接口唯一匹配、仅表唯一匹配、两者同时填写、无匹配、多个匹配、接口不符、仅存在覆盖网段或默认路由，以及默认 ens3 但目标精确路由位于 tailscale0 的情况。局部错误场景必须确认该规则不修改，其他独立有效规则仍可同步。

---

## 三十六、Tailscale

已经通过实际 Linux 系统验证：

```text
Local TCP Socket
     ↓
RPDB
     ↓
Table 52
     ↓
Peer /32 dev tailscale0
     ↓
congctl lock brutal
     ↓
TCP congestion control = brutal
     ↓
tailscale0
     ↓
Tailscale UDP encapsulation
```

因此 TCP Brutal 可以作用于本机通过 `tailscale0` 建立的 TCP Socket。

但 Forwarded TCP 不是 Ubuntu 本机 TCP Socket，因此 Linux Gateway 的 TCP congestion control 不能直接接管端到端被转发 TCP 连接。

---

### Rule / Route Lock 联动（已确认，第 17 项）

沿用 TCP Brutal v2.0.1 原行为：配置 `lock` 同时控制规则锁及 Route 拥塞算法锁。

- `lock` 缺省或 true：规则锁定，Route 使用 `congctl lock brutal`。
- `lock=false`：规则不锁定，Route 使用 `congctl brutal`，清除 `RTAX_LOCK` 中仅对应 `RTAX_CC_ALGO` 的位，不改变其他 metrics 锁位。
- 所有前文固定 `congctl lock brutal` 的示例均表示默认 lock=true；Route Desired、Diff、Repair、Verification 及管理记录必须依据实际配置 lock。
- Flush / 配置删除的属性清理按 Go 记录的算法和实际写入 lock 状态匹配；不无条件以 lock=true 作为匹配前提，不恢复旧算法或锁状态。
- 测试覆盖默认 true、显式 false、true/false 原位切换及无关 metrics 锁位保留。

---

## 三十七、Route 安全校验

指定 `table` 或 `interface` 任意一项时，禁止凭空创建不存在的 Peer Route；允许仅填写一项，按第三十五节的唯一精确匹配规则定位另一项。

必须验证：

```text
Route Exists
Table Correct
Interface Correct
Destination Correct
```

如果实际 Interface 与配置不符，直接报错，禁止擅自迁移 Route。

---

## 三十八、Go Route 实现

Go 对显式策略路由使用 Netlink 读取、验证及修改属性；未指定 interface / table 的普通路由委托 brutalctl。可以读取普通路由做诊断，但不重新实现官方生命周期。

建议封装：

```go
type RouteManager interface {
    Find(...)
    Validate(...)
    Repair(...)
}
```

Linux 实现使用成熟 Netlink 库或受控系统调用封装，具体依赖在实现阶段确认。

---

## 三十九、保留 Route 属性

不能只根据 Prefix 和 Interface 重建 Route。

必须保留现有：

- Gateway
- Device
- Source
- Metric
- Scope
- Protocol
- MTU
- Preference
- 其他 Netlink Route Attribute

目标仅是在当前 Route 基础上增加或恢复：

```text
congctl lock brutal
```

不得意外覆盖 Tailscale 或其他 Route Manager 管理的属性。

---

### Route 并发变更处理

Repair / Cleanup 写入前重新读取对应身份的 Route，使用最新无关属性构造变更，无法完整保留属性则跳过而不写入。检查发现路由身份改变时暂缓；写后回读验证。外部修改仍可能发生在读取与写入之间，不宣称 CAS 或跨管理器原子性；发现冲突留给下一触发重试。

---

## 四十、Route-Only Repair

如果 Brutal Desired 与 Current 一致，但 Go 管理的策略 Route 缺失目标拥塞算法或 lock 状态，禁止 Flush Brutal Ruleset；仅对显式 table / interface 对应 Route 执行 Repair。普通路由由 brutalctl 管理，不产生 Go Netlink Repair。

只执行：

```text
Route Repair
```

流程：

```text
Detect Route Difference
↓
Confirm Brutal Ruleset Unchanged
↓
Repair Route Attribute
↓
Re-read Route
↓
Verify
```

---

## 四十一、Diff 模型

Diff 分两层。

### Brutal Diff

比较：

```text
Prefix
Rate
Lock
Order
```

### Route Diff

比较：

```text
Table
Destination
Interface
Congestion Control
Lock
Required Route Attributes
```

建议：

```go
type Diff struct {
    Brutal BrutalDiff
    Routes []RouteDiff
}
```

必须能够区分：

```text
No Difference
Brutal Difference
Route-Only Difference
Both
```

---

## 四十二、Apply Plan

修改系统之前先生成 Apply Plan：

```go
type ApplyPlan struct {
    FlushBrutal       bool // 正常 Apply 为 false
    RecoveryRebuild   bool
    AddRules          []DesiredRule
    UpdateRules       []DesiredRule
    DeleteRules       []netip.Prefix
    RouteRepair       []RouteChange
    RouteCleanup      []RouteChange
    SkippedRules      []RuleIssue
    ProtectedPrefixes []netip.Prefix
    DeferredOrder     bool
    DomainGroups      []DomainUpdateGroup
    ControllerEffects []ControllerEffect // brutalctl 隐式 Route 操作
}
```

先计算：

```text
Current State
+
Desired State
↓
Diff
↓
Apply Plan
```

再执行。

Dry Run 直接输出 Apply Plan，但不能修改系统。

---

## 四十三、逐规则容错与可用性优先（已确认）

普通 `apply` 采用逐规则同步，不因某一条规则失败而停止其他独立规则；正常同步禁止全量 Flush 后重建，管理记录异常恢复按第 20 项执行明确例外。显式 `flush` 仍用于主动清空。

- JSON 语法错误、顶层配置或 DNS Resolver 配置非法、日志初始化失败、锁获取失败、Current State 无法可靠读取、权限或内核依赖不可用，属于全局错误：停止执行，不进行修改。
- 单条 Rule 的字段错误、Rate / Prefix 非法、DNS 查询故障、路由定位或安全校验失败，属于局部错误：记录并跳过该规则，继续处理其他规则。
- Domain Rule 作为整体处理：A / AAAA 存在最终查询故障时，不应用部分解析结果；保留可识别的旧展开规则。
- 相同 Prefix 参数冲突时，冲突涉及的所有规则都暂缓，不选择任何一方；不影响其余独立规则。
- 跳过规则不能被当作删除意图。通过持久化来源记录识别旧规则并保留；无法可靠识别失败规则旧来源时，只保护可识别来源，其余旧受管规则仍可按删除意图清理；报告无法关联的条目，不暂停全部删除。
- 扩展 BrutalController 支持单条 Delete 和原位 Add / Update。先执行有效规则的添加、更新和路由修复，再删除能够确认已从配置移除、且不属于失败规则保护范围的旧规则。
- 每条操作后重新读取验证；局部失败停止该规则的后续操作，记录实际状态并继续独立规则。运行中出现全局故障时停止后续系统修改。
- 逐条操作不能保证跨 Brutal / Route 的事务原子性；必须报告局部完成状态，不宣称回滚或完整同步。
- 执行计划遵循有效规则的最终数值排序，并根据内核插入方向确定实际操作方向；当底层接口无法无损调整 Kernel 顺序时，保留已有业务状态，报告顺序差异并暂缓重排，禁止通过 Flush 强行满足顺序。
- 来源记录存于 `/var/lib/tcp-brutal-config/`，包含已成功应用规则的 Prefix、参数与来源，采用原子文件替换；修改前持久化操作意图，修改后保存验证结果。单条持久化失败停止后续系统修改。
- 来源按配置文件绝对路径、源类型、源值以及重复源出现序号识别；不能仅使用会随重排变化的 SourceIndex。
- 本次含跳过、局部失败或暂缓重排时，返回 `7 = Partial Success / Degraded`；即使其余规则全部成功，也不能返回完整成功。

这里的可用性是失败隔离与尽量保留已有规则，不保证网络链路或业务端到端可用。

---

## 四十四、修改前校验与失败隔离

先完成全局配置校验，再在系统修改锁外逐规则解码、规范化和实时 DNS，生成有序候选。获取锁后重新读取 Current 与管理记录，执行冲突检测、Stable Dedup、Route Safety Validation，生成含有效操作、跳过原因和保护范围的最新 Apply Plan。Run 非 DNS 事件使用本进程成功解析结果。

所有将执行的操作必须通过对应校验；局部错误仅禁止对应规则修改，全局错误禁止整次修改。禁止把失败规则从 Desired 中消失解释为需要删除其旧状态。

ApplyPlan 增加逐条 Add / Update / Delete、SkippedRules、ProtectedPrefixes、UnidentifiedSources 和 DeferredOrder 信息；普通 Apply 的 FlushBrutal 必须为 false；管理记录异常恢复计划明确标记全量重建与跨表 brutal 属性清理，FlushBrutal 为 true。Dry Run 展示相同计划及退化原因，不修改系统或持久化记录。

---

## 四十五、Post-Apply Verification

Apply 后必须重新读取系统状态，不能使用 Apply 前缓存的 Current State。

重新读取：

```text
/proc/net/tcp_brutal/rules
Linux Route
```

验证：

```text
Desired Brutal
=
Current Brutal
```

以及：

```text
Required Route State
=
Current Route State
```

顺序必须参与验证。局部失败或暂缓重排时，验证有效操作和保护范围并报告剩余差异，结果为退化状态，不能宣称 Desired 与 Current 完全一致。

---

## 四十六、CLI

Go Binary：

```text
tcp-brutal-config
```

支持：

```text
tcp-brutal-config run
tcp-brutal-config apply
tcp-brutal-config apply --dry-run
tcp-brutal-config check
tcp-brutal-config diff
tcp-brutal-config list
tcp-brutal-config status
tcp-brutal-config flush
tcp-brutal-config flush --dry-run
```

### Settings 模块分组与默认值（已确认）

`settings` 内按模块组织字段。完整默认配置为：

```json
{
  "settings": {
    "dns": {
      "query_interval": "60s",
      "timeout": "5s"
    },
    "config": {
      "debounce": "500ms",
      "rescan_interval": "60s"
    },
    "route": {
      "debounce": "500ms",
      "resync_interval": "60s",
      "operation_timeout": "10s"
    },
    "brutal": {
      "poll_interval": "30s",
      "operation_timeout": "10s"
    }
  }
}
```

- 未配置 `settings`、`settings: {}` 或 `settings: null` 时，使用全部默认值。
- 模块省略、模块为空对象或 null 时，使用该模块默认值；仅配置部分字段时，仅覆盖这些字段。
- 单个字段缺省、null、空字符串或仅空白字符串时使用默认值；非空值按正数 Go Duration 校验，拒绝零、负数、错误类型及非法文本，不静默纠正错误值。
- 未知模块或未知字段属于全局配置错误，避免字段拼错后静默失效。
- 每次加载 / 重载都从全套默认值开始，再覆盖本次配置；删除设置字段会恢复默认值，不继承上一版的自定义值。
- DNS Query Interval 只控制定时 DNS 查询；Config 采用父目录文件事件监听与独立内容核对，Route 采用 Netlink 监听与独立全量核对，Brutal 独立读取实际规则。不将各周期混为全局同步间隔。
- Config Debounce 用于合并配置保存事件，Route Debounce 用于合并路由事件。Rescan / Resync 用于漏事件兜底；Brutal Poll 是规则状态检查，不比较 MEMBERS / SENT 动态统计。
- 新增 `settings.dns.timeout` 默认 `"5s"`，`settings.route.operation_timeout` 和 `settings.brutal.operation_timeout` 默认 `"10s"`，适用相同缺省与空值默认规则。超时分别约束单次查询和单次系统操作；重试最多三次、间隔三秒仍固定，未加入重试与并发设置字段。

测试覆盖 Settings 缺省 / 空对象 / null、模块缺省 / 空对象 / null、字段缺省 / null / 空白、部分覆盖、非法 Duration / 类型 / 未知字段，以及重载时删除字段恢复默认值。

### 参数与 Help（已确认）

- Help、Usage、示例及 CLI 错误提示中的程序名使用运行时变量 `program`，通过 `filepath.Base(os.Args[0])` 获取用户调用的文件名（保留扩展名）；若为空则回退到 `filepath.Base(os.Executable())`，两者均不可用时回退为 `tcp-brutal-config`。
- 文档 Help 模板中的 `{program}` 是渲染占位符，实际输出必须替换为自身调用文件名，不输出占位符本身。重命名 Binary 或通过命名符号链接调用时显示对应调用名称。
- 项目名称、默认配置目录、日志及锁文件路径保持既有约定，不随 Binary 重命名改变；systemd ExecStart 使用安装时确定的实际可执行路径。

- `--config` / `-c`：配置路径，默认 `/etc/tcp-brutal-config/config.json`；支持位于子命令之前或之后，重复指定时拒绝，避免歧义。
- `--dry-run` / `-n`：仅 Apply / Flush 支持，完整预览但不修改。
- `--help` / `-h`：顶层与所有子命令支持；`help [command]` 同样支持。
- `--version` / `-v`：顶层显示版本；提供 `version` 命令，包含版本、提交及构建时间。
- 无参数显示顶层 Help 并返回 0，不隐式 Apply；未知命令 / 参数、不支持的参数组合返回 2。
- Help / Version 不加载配置、不初始化文件日志、不查询 DNS、不获取锁，不要求 root 或 Brutal 内核模块。
- List 不要求配置；Flush 从管理记录定位需要清理的路由，不依赖实时 DNS 或配置中的规则列表。

顶层 Help 计划输出：

```text
Usage:
  {program} <command> [options]

Commands:
  run       持续同步 DNS、配置与实际规则
  apply     同步规则与路由属性
  check     校验配置及规则，检测状态差异
  diff      显示规则、顺序及路由差异
  list      显示当前 Brutal 规则
  status    显示配置与同步状态
  flush     清空 Brutal 规则并清理 Go 添加的匹配路由属性
  version   显示版本与构建信息
  help      显示帮助，可指定子命令

Options:
  -c, --config <path>  配置文件（默认 /etc/tcp-brutal-config/config.json）
  -h, --help           显示帮助
  -v, --version        显示版本

Command options:
  -n, --dry-run        仅 apply / flush：预览，不执行修改

DNS refresh interval:
  JSON settings.dns.query_interval（默认 "60s"），有效重载后更新；仅控制 DNS 定时解析

Examples:
  {program} run -c config.json
  {program} apply -c config.json -n
  {program} flush -n
  {program} help run
```

子命令 Help 显示对应 Usage、用途、适用参数、默认值和示例，不展示其他子命令不适用的参数。Flush Help 明确提示清空范围及 Run 运行时会重新同步；Run Help 明确间隔来自 JSON、退出不清空规则。

测试覆盖 Binary 重命名、带路径及符号链接调用时的程序名替换、无占位符泄漏、长短参数等价、配置路径参数位置、重复参数拒绝、Help / Version 无系统依赖和无修改、错误参数退出码、interval 默认 / 校验 / 动态重载。

---

## 四十七、check

`check` 必须执行：

- Config Parsing
- Log Config Validation
- DNS Config Validation
- Rule Validation
- Rate Validation
- IP / Prefix Validation
- DNS 实时查询
- Domain Expansion
- Conflict Detection
- Stable Dedup
- Desired State Generation
- Current State Parsing
- Route Validation
- Difference Detection

不修改系统。

---

## 四十八、diff

`diff`：

```text
重新生成 Desired State
↓
读取 Current State
↓
显示 Brutal Diff
↓
显示 Route Diff
```

必须基于本次最新 DNS 查询结果。

输出必须明确区分：

```text
ruleset
order
dns-derived
route
```

---

## 四十九、apply

`apply`：

```text
实时重新解析 DNS
↓
生成 Desired State
↓
读取 Current State
↓
计算 Diff
```

如果 No Difference，不执行任何修改。

如果 Route-Only Difference，仅 Route Repair。

如果 Brutal Difference，则逐规则增量同步；局部失败隔离，顺序无法无损调整时暂缓重排。

---

## 五十、Dry Run

`apply --dry-run` 必须完整执行：

- Config Validation
- DNS Query
- Desired State
- Current State
- Diff
- Apply Plan

但不能执行 Flush、Add、Route Replace 或 Route Repair。

输出：

```text
would_flush
would_add
would_remove
would_reorder
would_repair_route
```

---

## 五十一、list

显示 Current Brutal State，可同时提供：

```text
Prefix
Rate
Gain
Lock
ID
Members
Sent
```

不得修改状态。

---

## 五十二、status

显示综合状态：

```text
Version
Config Path
Log Configuration
DNS Resolver Configuration
Brutal Current State
Managed Route State
Synchronization Status
```

如果状态不同步，显示摘要但不修改系统。

---

## 五十三、flush

危险操作：

```text
tcp-brutal-config flush
```

必须明确该操作会清空 TCP Brutal Ruleset。

Dry Run：

```text
tcp-brutal-config flush --dry-run
```

只显示当前规则和预计操作。

---

### Go 管理的策略路由属性清理（已确认）

`tcp-brutal-config flush` 清空 Brutal Ruleset，并清理 Go 添加到已有策略 Route 上的属性。清理规则为：匹配 Go 写入的属性值则移除，不匹配则跳过；不恢复修改前的旧值。

- 持久化记录 Go 实际添加的属性、写入值及对应 Route 身份（Table、规范化 Destination、Interface，以及区分同前缀路由所需的身份字段）。记录不依赖当前配置，配置变更后仍可执行 Flush 清理。
- 原本已存在且相同的属性不算 Go 添加，不登记为可清理属性。
- Apply 前校验并持久化管理记录，才允许添加属性；重复 Apply 不丢失已有归属记录。
- 每次 Apply 重新读取路由；已有 Route 的目标属性缺失时执行 Route-Only Repair，不 Flush Brutal Ruleset。整条 Route 不存在时仍按安全规则报错，不创建 Peer Route。
- Flush 持有独占锁，重新读取对应 Route；当前属性及其关联的 lock 位与 Go 记录的写入值一致时，移除 Go 添加的属性及关联 lock 位。
- 不匹配、属性已不存在或 Route 已不存在时跳过，不恢复旧属性、不覆盖第三方设置、不重建 Route；不匹配本身不视为错误。
- 清理拥塞控制属性及其 lock 位时按关联整体匹配，不能只因算法名称匹配就删除已被外部改变的 lock 状态。
- 清理时保留 Route、Table、Interface、Gateway、Source、Metric、Protocol、MTU 等其他属性；不删除 Peer Route、整个 Table 或 RPDB / `ip rule`。
- 清理后重新读取验证；已完成或无需清理的记录移除，实际清理失败的记录保留以便重试，并返回失败。
- `flush --dry-run` 输出将清空的规则、将移除的 Go 属性及将跳过的条目，不修改系统或管理记录。
- 直接执行 `brutalctl flush` 不执行 Go 的属性清理流程。

测试必须覆盖：匹配时移除、不匹配时跳过、原本已存在的相同属性不移除、属性已缺失、Route 已删除、关联 lock 位已变化、配置已变更、重复 Apply 后 Flush、清理失败重试、Dry Run 无修改，以及清理后其他 Route 属性不变。

---

### 管理记录异常恢复（已确认）

- 使用独立初始化标记检测已初始化后记录丢失；若标记和记录都不可用，无法区分首次安装，按首次安装处理，不自动清空未知状态。损坏记录保留为诊断资料，采用带版本与校验的原子记录文件。
- 恢复前完整解析当前 JSON、完成全局校验、获取系统修改锁，生成有效规则及局部错误计划；全局错误不开始恢复。
- 持久化恢复阶段记录后清空全部 Brutal 规则，并枚举当前网络命名空间所有表，清除当前算法为 brutal 的 Route 算法属性及对应锁位，保留其他字段。所有权丢失时无法保护外部 brutal 状态，明确记录此恢复范围。
- 完成清理后逐条重建当前有效配置，并生成新的管理记录；局部配置 / DNS 错误仍跳过，恢复已清空的旧失败规则不保证可还原，需明确报告。
- 恢复阶段记录包含阶段、配置内容版本、条目归属和验证结果。清理失败只重试失败条目；重试前重读并检查最新需求，已由新有效规则使用的 Route 取消旧清理。已开始重建后不重复全量 Flush，配置变化则重新计算未完成计划，避免使用过时计划。
- Dry Run 只展示恢复原因、全量清理范围和重建计划，不写标记、记录或系统状态。显式 Flush 与异常恢复均按局部失败继续、全局故障停止的规则执行。
- 测试覆盖首次无记录、初始化后缺失、损坏、全局校验失败、外部 brutal 清理、无关 Route 属性保留、局部清理失败、恢复断点、部分重建及 Dry Run。

---

## 五十四、并发锁

Go 版需要防止 systemd、cron、manual execution、multiple CLI 同时 Apply。

使用 Process-Level File Lock。

Linux 可以基于 `flock` syscall 通过 Go 封装实现，例如：

```text
/run/tcp-brutal-config.lock
```

Apply / Flush 必须获取独占锁。

---

## 五十五、Context 与信号

Go 程序必须处理：

```text
SIGINT
SIGTERM
```

使用：

```go
context.Context
```

传递给：

- DNS
- Retry Timer
- brutalctl
- Route Operation
- Apply
- Verification

systemd Stop 时必须能安全退出。

---

## 五十六、Exit Code

建议：

```text
0 = Success / No Difference
1 = Runtime Failure
2 = Configuration Error
3 = Difference Detected
4 = Verification Failure
5 = Permission Error
6 = Dependency / Kernel Support Error
7 = Partial Success / Degraded
```

具体编码在实现阶段固定。

---

## 五十七、错误模型

内部优先使用 `error`，配合：

```go
errors.Is
errors.As
fmt.Errorf("%w")
```

关键模块可定义：

```text
ConfigError
DNSError
RuleConflictError
RouteError
ApplyError
VerificationError
```

CLI 最终统一转换为 Log、stderr 和 Exit Code。

---

### 逐规则容错验收

必须覆盖：有效 IP + 错误 Rule + 有效 IP；Domain 解析失败保留旧地址而其他规则更新；A 成功 AAAA 失败不部分应用；相同 Prefix 冲突隔离；Route 不符隔离；单条 Add / Route Repair / Delete 失败后继续独立操作；未知失败来源不抑制其他旧受管规则删除；配置重排不导致错误归属；无法无损重排时保留业务状态并报告差异；全局失败不修改；Dry Run 无修改；部分完成退出码为 7；正常 Apply 不调用 Flush，管理记录异常恢复单独测试全量清理与重建。

---

## 五十八、测试策略

至少包含：

```text
Unit Tests
Integration Tests
Linux Integration Tests
```

核心逻辑不能只能通过真实服务器测试。

---

## 五十九、顺序测试

- IPv6 写在 JSON 前面时，最终 IPv4 仍在前。
- IPv4 `.2` 排在 `.10` 前，禁止字符串字典排序。
- IPv6 按二进制地址数值排序，压缩写法不影响结果。
- 主机地址先 Masked，再按网络地址排序；同网络地址 `/24` 排在 `/32` 前。
- IP / Domain / 多地址混合输入最终统一排序，Domain Group 可被打散，来源仍完整。
- 改变业务 JSON 顺序不产生最终规则顺序变化；DNS 选择规则仍首次命中优先。
- Current 不排序，受管顺序不同必须报告差异；无法无损重排时保留状态并报告退化。

---

## 六十、Stable Dedup 测试

输入：

```text
A
B
A
C
B
```

结果：

```text
A
B
C
```

顺序不能变化。

---

## 六十一、冲突测试

输入：

```text
A 200Mbps
A 500Mbps
```

必须报错，不能静默选择其中一条。

---

## 六十二、Current Order 测试

```text
Desired:
A
B
C

Current:
A
C
B
```

必须判断为 DIFF。

---

## 六十三、Apply Order 测试

必须验证：

```text
Desired State
=
/proc/net/tcp_brutal/rules
=
brutalctl list
```

尤其测试 IP、Domain、IPv6、Domain Multi-Address 混合配置。

---

## 六十四、DNS 实时解析测试

第一次：

```text
example.com → 192.0.2.10
```

第二次：

```text
example.com → 198.51.100.10
```

第二次 Desired State 必须使用 `198.51.100.10`，不能使用旧地址。

---

## 六十五、DNS Failure 测试

```text
DNS Resolver Failure
↓
Retry × 3
↓
Fallback
↓
最终失败
```

必须验证：

```text
该 Domain Rule 跳过并保护旧规则
其他独立有效规则继续同步
最终返回退化退出码 7
```

---

## 六十六、DNS Fallback 测试

覆盖：

```text
DoQ → DoH
DoH → DoT
DoT → System
```

并确认 NXDOMAIN / NODATA 不会触发 Fallback。

---

## 六十七、Route-Only Repair 测试

Current：

```text
Brutal = Desired
```

但 Route missing `congctl lock brutal`，必须 Repair Route，并确认没有调用 Flush。

---

## 六十八、Route Safety 测试

配置：

```text
table=52
interface=tailscale0
```

实际：

```text
table=52
interface=ens3
```

必须记录该规则错误并跳过，验证该规则的 Route 与 Brutal 旧状态保留，其他独立有效规则仍可同步。

---

## 六十九、日志测试

必须覆盖：

```text
debug
error
warning
default
```

验证：

- Debug 输出详细信息
- Error 不包含正常运行日志
- Warning 包含 Warning + Error
- Default 包含正常摘要 + Warning + Error
- enabled=false 不参与文件日志；level=none 为非法配置
- Fatal Error 永远能够在 stderr 看见
- 标签不重复
- Timestamp 正确
- Component 正确

---

## 七十、日志路径测试

覆盖：

```text
Absolute Path
Relative Path
Missing Parent
Permission Denied
```

验证：

- 相对路径以 Config Directory 为基准
- 最终路径确定
- Append 生效
- 不会静默写错目录
- Permission Error 不会进入 Apply

---

## 七十一、日志初始化失败测试

```text
Logging Enabled
↓
Log File Cannot Open
↓
Apply Abort
```

确认 Brutal Ruleset 与 Route 都不变。

---

## 七十二、Dry Run 测试

Dry Run 必须验证：

```text
执行 DNS
生成 Desired
读取 Current
生成 Diff
生成 Apply Plan
```

同时确认：

```text
没有实际 Flush
没有 Add
没有 Route Modification
```

---

## 七十三、Concurrency 测试

两个 Apply 同时运行时，只能一个获得写锁。

另一个等待或明确失败，不能并行修改 TCP Brutal Ruleset。

---

## 七十四、Run 持续同步与 Systemd（已确认）

新增常驻命令：

```text
tcp-brutal-config run --config /path/to/config.json
tcp-brutal-config run -c /path/to/config.json
```

### 持续检测

- 启动时立即执行一次完整同步，后续复用逐规则增量 Apply、Route-Only Repair、来源记录和修改后验证，正常同步不执行全量 Flush，管理记录异常恢复按恢复流程例外处理。
- `settings.dns.query_interval` 仅控制定时 DNS 记录解析，为正数 Go Duration 字符串，默认 `"60s"`；省略 `settings`、`settings.dns` 或 `query_interval` 时使用默认值，空字符串 / 空白 / null 使用默认值；零、负数、错误类型或非空非法值属于全局配置错误。CLI 不提供 `--interval`。有效重载后更新 DNS 计时器，当前查询不被中断。定时重新解析业务 Domain 与 Bootstrap Endpoint，记录变化时触发对应规则增量更新；它不是全局状态同步或 Route / Brutal 检查间隔。
- DNS 不存在通用服务端变更通知接口，本版通过轮询检测地址变化；不实现 DNS 推送或常驻 DNS 缓存。
- 监听配置父目录并过滤目标路径，支持文件原位修改和原子替换；按 `settings.config.debounce` 合并事件，读取校验成功后采用新快照。按 `settings.config.rescan_interval` 独立核对内容；事件溢出或监听失效时重新建立监听并核对。
- 通过 Netlink 订阅路由变化，按 `settings.route.debounce` 合并事件后仅触发 Go 管理的策略 Route 重新读取与修复，不重复查询 DNS；自身修改产生的事件经 Diff 判断无差异，不重复写入。订阅失败或断开时记录告警；按 `settings.route.resync_interval` 独立全量核对并尝试恢复订阅，不复用 DNS 间隔；订阅正常时仍保留该兜底核对。
- 外部直接修改 Brutal 规则没有本版专用事件订阅；按 `settings.brutal.poll_interval` 独立读取并比较规则，不把 DNS 计时器定义为规则检查计时器。Run 的非 DNS 事件使用本进程最近成功结果核对对应实际状态，不重新查询 DNS。
- Domain 地址成功变化时，先添加 / 更新新地址及 Route 并验证；任一新地址操作失败时保留已成功新地址和整组旧地址，直到整组成功才清理过期旧地址。成功整体无地址按删除意图清理；其他独立规则继续。

### Link 事件

订阅 Netlink Link 变化，按 Route debounce 合并接口删除、重建及改名事件，重新核对对应 Interface 与 Route，按名字重新确定 ifindex；不可定位时执行局部保护。

### 生命周期与并发

- 每轮同步只运行一个任务；期间收到的新事件合并为下一轮请求，不并行执行，不堆积无限事件。DNS 计时器以本批 DNS 查询及对应更新完成后间隔计时，避免慢 DNS 导致查询任务重叠；配置和 Route 事件独立触发。
- Run 使用单独非阻塞实例锁 `/run/tcp-brutal-config-run.lock`，同一网络命名空间内仅允许一个 Run。系统修改锁 `/run/tcp-brutal-config.lock` 仅在每轮读取 Current / 生成计划 / 修改 / 验证期间持有，等待事件和 DNS 查询时不占用；竞争失败本轮暂缓，下次触发重试。
- 手动 Apply / Flush 仍使用系统修改锁。Run 存活时手动 Flush 清空的规则会在后续同步恢复；需要持续保持清空时应先停止 Run 再 Flush。
- SIGINT / SIGTERM 取消查询、重试和事件监听，停止调度下一轮；保留已有规则及 Route 属性，不自动 Flush。已发生的修改按实际状态记录，不承诺信号退出回滚。
- `run` 不支持 `--dry-run`；预览使用 `apply --dry-run`。

### 错误与重载

- 启动时全局配置错误或依赖不可用：按现有退出码退出；单条规则错误返回本轮退化结果，但 Run 保持运行并在后续轮次重试。
- 运行中配置文件缺失、暂时不可读或全局配置错误：拒绝该版本，保留最后有效配置继续同步，并记录错误；仅局部规则错误时接受配置快照并执行失败隔离。
- DNS / 单条操作失败不会结束 Run；Current 读取、权限、依赖或日志写入等全局故障禁止本轮系统修改，向 stderr 报告并通过对应模块的后续触发或独立重试调度处理，不宣称本轮成功。
- 每轮记录触发原因、成功 / 跳过 / 失败条数、DNS 和 Route 差异及退化状态；SIGINT / SIGTERM 正常结束返回 0，不能用最终退出码代替运行期间健康状态。

### Systemd

提供 `Type=simple` Service，`ExecStart` 使用 `tcp-brutal-config run --config ...`，配置 `Restart=on-failure`、`RestartSec=5s`，网络就绪后启动；Tailscale 场景可添加对应服务顺序依赖。保留独立的一次性 Apply 使用方式。

### 验收

覆盖启动同步、DNS 地址变化、DNS 故障保护旧规则及恢复后同步、配置增加 / 删除 / 原子替换、非法配置保留最后有效版本、局部错误不阻断其他规则、Netlink 属性清除后自动修复、订阅断线降级及恢复、外部 Brutal 变化、事件合并和无差异无写入、手动命令锁竞争、Flush 后恢复、信号退出无清空、重复 Run 拒绝及慢查询取消。

---

## 七十五、依赖原则

Go 正式版目标：

**尽量减少外部命令。**

内部原生处理：

```text
JSON
DNS
IP
Prefix
State
Diff
Logging
Lock
Route Query
Route Modification
```

不依赖：

```text
jq
getent
awk
sed
grep
sort
```

TCP Brutal 操作第一版依赖 `brutalctl` 作为官方控制接口，同时需要它调用的 `ip`（iproute2）；普通路由操作在计划中标注为委托 brutalctl，策略 Route 修改单独纳入 Go 计划。

---

## 七十六、排序与去重边界

旧方案禁止 Desired 排序是为了保持 JSON 顺序；该要求已由第 32 项替换为确定性的数值排序。禁止排序隐藏冲突，禁止从 Map 遍历输出最终顺序，禁止对 Current 排序后 Diff。

正确路径：

```text
有序配置 / 逐条校验与 DNS
→ Prefix 规范化 / 保存全部来源
→ 冲突检测
→ Stable Dedup 与来源合并
→ IPv4 优先、网络地址与 CIDR 长度升序
→ Ordered Desired
```

Current 直接读取原始 Kernel 顺序，在受管范围中与 Desired 对比；外部规则保留，无损重排限制与异常恢复例外按已确认约束执行。

---

## 七十七、最终数据流

```text
加载配置 / 分层逐条校验 / 准备 Logger
↓
锁外 DNS（Run 非 DNS 事件使用本进程成功结果）
↓
候选规范化 / 追踪全部来源
↓
获取系统修改锁
↓
重新读取 Brutal / Route / 管理记录
↓
冲突检测 / Stable Dedup / 来源合并 / IPv4 与数值排序
↓
Route Safety 与 ControllerEffects 校验
↓
生成 Apply Plan（含 Domain 更新组与保护范围）
↓
Dry Run → 输出计划，不写记录或系统
↓ 非 Dry Run
管理记录异常？→ 已确认的恢复阶段流程
↓ 正常同步
持久化操作意图
↓
逐条 Add / Update + 对应 Route Repair
↓
逐项回读验证 / 汇总 Domain 更新组
↓
仅成功更新组释放旧保护；失败组保留成功新项和整组旧项
↓
安全 Delete / RouteCleanup（重读并匹配，保留共享需求）
↓
最终回读验证 / 持久化实际结果
↓
输出成功 / 差异 / 退化 / 全局错误；释放锁
```

局部失败继续独立条目；运行中全局故障停止后续修改，保留可记录的实际结果，不宣称事务回滚。显式 Flush 使用单独的全量规则清理与匹配 Route 清理流程。

---

## 七十八、开发流程

项目开发严格遵循：

```text
计划
 ↓
实现
 ↓
核对
 ↓
编译
 ↓
发版
```

当前状态：

```text
[✓] 计划
[ ] 实现
[ ] 核对
[ ] 编译
[ ] 发版
```

---

## 七十九、实现阶段目标

进入实现阶段后，优先完成：

```text
1. Go Module 和项目目录
2. Config Struct
3. Config Validation
4. Logger
5. net/netip Rule Normalization
6. Rate Parser
7. DNS Resolver Interface
8. System Resolver
9. DoH
10. DoT
11. DoQ
12. Retry / Fallback
13. Domain Expansion
14. Stable Dedup
15. Conflict Detection
16. Desired State
17. Current Brutal State Parser
18. Ordered Diff
19. Route Manager
20. Apply Plan
21. Brutal Controller
22. 逐规则增量 Apply 与局部失败隔离
23. Route-Only Repair
24. Verification
25. CLI
26. Run 持续同步、配置轮询与 Netlink 监听
27. Unit Tests
28. Integration Tests
```

完成实现后停止，等待进入核对阶段确认。

---

## 八十、最终原则

### Rule

```text
JSON 顺序
↓
实时 DNS
↓
Domain 展开并追踪来源
↓
Stable Dedup / Merge Sources
↓
Sort IPv4 First / Address / CIDR
↓
Ordered Desired State
```

### DNS

```text
每次执行重新解析
↓
默认不持久化缓存
↓
Resolver Failure 才 Fallback
↓
NXDOMAIN / NODATA 不 Fallback
↓
DNS Failure 仅跳过失败 Domain 并保护旧状态，其他有效规则继续同步
```

### Diff

```text
内容
+
参数
+
顺序
+
Route
```

全部参与状态判断。

### Apply

```text
有 Brutal Difference
→ 逐规则增量同步 / 局部失败隔离

只有 Route Difference
→ Route-Only Repair

没有 Difference
→ No Operation
```

### Safety

```text
所有 Validation 完成
↓
才允许修改系统
```

### Logging

```text
统一 Logger
↓
debug/error/warning/default
↓
相对路径以 Config Directory 为基准
↓
Append
↓
结构化字段
↓
适度 [LEVEL] [COMPONENT]
↓
Fatal Error 永不隐藏
```

### Architecture

```text
Go Native
↓
强类型
↓
有序数据模型
↓
低 Shell 依赖
↓
可测试
↓
可维护
↓
可发布
```

**TCP Brutal Config Go = 最终正式实现。**
