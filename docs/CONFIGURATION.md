# 配置参考（1.0.0）

配置为严格 JSON，不支持注释或未知字段。顶层错误、非法 Settings、非法或重名 Resolver 拒绝整个版本；业务 rules 和 dns.rules 逐条解析，局部错误隔离。rules 必须为非空数组，主动清空使用 flush。

## 完整示例

```json
{
  "log": {
    "enabled": true,
    "level": "default",
    "path": "/var/log/tcp-brutal-manager.log"
  },
  "dns": {
    "resolvers": [
      { "name": "cloudflare", "type": "DoH", "address": "cloudflare-dns.com" },
      { "name": "system", "type": "System" }
    ],
    "rules": [
      {
        "domain": ["full:example.com", "domain:example.net"],
        "resolver": "cloudflare",
        "fallback": false
      }
    ]
  },
  "rules": [
    { "ip": "192.0.2.1", "cidr": 32, "rate": 200 },
    { "domain": "example.com", "cidr": { "v4": 24, "v6": 64 }, "rate": "350Mbps" },
    { "ip": "100.64.0.0", "cidr": 10, "rate": 200, "noroute": true }
  ],
  "settings": {
    "dns": { "query_interval": "60s", "timeout": "5s" },
    "config": { "debounce": "500ms", "rescan_interval": "60s" },
    "route": { "debounce": "500ms", "resync_interval": "60s", "operation_timeout": "10s" },
    "brutal": { "poll_interval": "30s", "operation_timeout": "10s" }
  }
}
```

示例地址和域名需要替换为自己的目标。未匹配 DNS Rule 时使用首个 Resolver；无 resolvers 时使用系统解析。末项 /10 示例不创建普通路由，适用于希望保留现有隧道节点路由的场景。

## 业务 rules

每条规则填写一个 ip 或 domain。ip 使用纯 IP，cidr 单独填写；业务 domain 是待解析的单个主机名，不接受 DNS 选择模式或数组。

| 字段 | 行为 |
|---|---|
| rate | 必填；数字或无单位字符串默认 Mbps；支持 bps、Kbps、Mbps、Gbps，区分大小写，不支持 Byte 单位。精确换算后必须为正整数 Mbps，上限 4294967295。200、"200Mbps"、"200000000bps" 等价；"2.5Gbps" 为 2500；"2.5Mbps" 被拒绝。 |
| cidr | 省略时 IPv4 /32、IPv6 /128。IP 场景使用整数；Domain 可用整数或 {"v4":24,"v6":64}，未给出的族使用最大长度。有效范围 v4 0–32、v6 0–128；显式 0 有效；null 不是缺省。主机位清零。 |
| lock | 布尔值，省略默认 true；同时控制规则和 Go 写入的拥塞算法锁位。 |
| noroute | 默认 false。true 将 noroute 传给 brutalctl，不建立普通路由。 |
| route_strict | 默认 false。true 仅匹配精确 CIDR；只约束 Go 管理路由，不影响 Brutal 大段规则。 |
| table | 路由表号。指定后定位该表中 CIDR 范围内的全部已有路由，并强制 noroute。 |
| interface | 接口名。指定后跨表定位该接口上 CIDR 范围内的全部已有路由，并强制 noroute；与 table 同时填写时两者必须匹配。 |

同 Prefix 参数完全相同时去重并保留全部来源；全部来源都不再需要时才删除。相同 Prefix 参数冲突会隔离相关条目；不同 Prefix 允许重叠。Desired 按 IPv4→IPv6、网络地址数值升序、同地址 CIDR 长度升序排列。Current 保留内核顺序。外部复用规则不认领、不参与受管顺序比较。

指定 table/interface 不会创建路由或 ip rule。默认处理同地址族且完全包含于业务 CIDR 的全部路由，包括精确网段与更具体子网；不处理更大的覆盖网段。`route_strict` 默认 false，true 时只匹配精确 CIDR，允许多个精确候选。无候选时 Brutal 规则仍维护并报告退化。多路径路由指定 interface 时要求全部下一跳符合接口。重叠业务 CIDR 由更具体者决定 Route lock；各 Brutal 规则保留自身速率。逐 Route 失败隔离，成功项不回滚。未指定两者时普通路由由 brutalctl 管理，错误可能发生在规则已写入之后，应核对实际状态。

## DNS Resolver 与选择规则

Resolver 的 name 必须唯一，type 精确填写 System、DoH、DoT 或 DoQ。

| type | 字段与默认值 |
|---|---|
| System | 只填写 name/type；不填写 address、port、path。 |
| DoH | address 为主机名，不含 scheme/path；固定 HTTPS；port 默认 443；path 默认 /dns-query。拒绝重定向；TLS 验证服务器名称和证书。 |
| DoT | address 为主机名，port 默认 853，不接受 path。Cloudflare 示例主机名为 one.one.one.one。 |
| DoQ | address 为主机名，port 默认 853，不接受 path。AdGuard 示例为 dns.adguard-dns.com。 |

加密 Resolver 的主机名先用系统解析进行引导。当前实现选用引导结果的首个地址，不承诺自动尝试每个地址。

dns.rules 的 domain 支持字符串或数组，resolver 引用 name，fallback 默认 false。按数组顺序选第一条命中规则：

| 模式 | 含义 |
|---|---|
| example.com 或 full:example.com | 精确匹配。 |
| domain:example.com | 自身及任意层级子域名，按域名标签边界匹配。 |
| keyword:example | 包含关键词。 |
| regexp:表达式 | 正则匹配。 |

匹配忽略域名大小写和末尾点。使用这些四种格式，不使用 *.example.com。非法正则或不存在的 resolver 引用会跳过该选择规则并报告；后续规则继续，否则使用首个 Resolver。

查询故障时每个 Resolver 最多尝试 3 次，间隔 3 秒。fallback=true 时按 resolvers 顺序尝试其他服务器，跳过已尝试项；NXDOMAIN / NODATA 不触发回退。有效空结果清理该来源旧规则；一族有地址、另一族 NODATA 正常应用；任一族最终故障则保留旧状态，不应用半份结果。新增地址组部分失败时保留已成功的新项及整组旧项，全部新项成功并验证后才删除过期项。

## Settings 与日志

上面的 Settings 示例列出全部默认值。Settings / 模块省略、null、空对象均使用默认值；单个 Duration 缺省、null 或空白使用默认值，非空值必须为正的 Go Duration 字符串。重载从默认值重新覆盖，删除字段恢复默认值。query_interval 是定时 DNS 解析周期，不是总同步周期，也不按 DNS TTL 自动调度。

log.enabled 默认 true；关闭文件日志用 false。level 支持 default、debug、warning、error，没有 none。default/debug 允许现有 INFO 日志，warning 过滤 INFO，error 仅记录 ERROR；debug 当前不增加独立的逐包 DNS 追踪。path 默认 /var/log/tcp-brutal-manager.log；相对路径以配置目录为基准。日志目录需要事先存在；文件追加写入，程序不负责轮转。
