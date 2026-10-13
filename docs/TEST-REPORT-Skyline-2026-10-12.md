> 本报告的业务地址、业务域名与主机标识已替换为文档示例或占位符；速率与行为结论保持。

# TCP Brutal Manager 1.0.0-Beta：Skyline 模块功能测试报告

日期：2026-10-12（Asia/Shanghai）。本报告记录本轮实际运行测试及最终核对，不代表全部极端故障或性能验收。

## 结论

已完成配置、规则编译、DNS、Brutal 控制、路由、监听、日志、管理记录恢复和 CLI 的一轮模块测试。主要受测行为符合预期；发现一个顺序比较缺陷及一个发布构建信息问题。测试完成与缺陷修复是不同阶段，本轮未修改源码或重新发布。

最终 check、diff、status、list 均退出 0；状态校验有效，受管记录 5 条、Go 路由属性记录 0 条、恢复阶段为空。原 5 条业务规则已恢复并保留。无 run 测试进程、无 tcpbmtest 临时接口，表 20260 / 20261 无测试路由。

## 环境与产物

- 主机：<test-host>，Ubuntu 26.04，Linux amd64，内核 7.0.0-38-generic。
- TCP Brutal 模块：2.0.1；控制器使用官方 brutalctl；Route 操作使用 Netlink。
- 安装目录：/usr/local/tcp-brutal-manager；程序为 tcp-brutal-config。
- 发布压缩包 SHA256：a0b9844455b806596d12ab4d45cefa6705adcb1e08b9180c2ffc5ba20289468c。
- 安装二进制 SHA256：92eef2c67019bbfbe14c1fe81b3202fb5d2d49ba578527c97cc978f6e707b055。
- version 输出：1.0.0-Beta commit=dev built=unknown。
- 当前测试配置：/tmp/tcpbm-skyline-config.json（临时路径，未安装正式服务配置）。
- 日志：/usr/local/tcp-brutal-manager/test.log，保持启用。
- 管理记录：/var/lib/tcp-brutal-config/state.json；异常恢复前备份为 /usr/local/tcp-brutal-manager/state-before-recovery.json，该备份仅供取证，不是当前有效状态。

## 当前有效配置

```json
{
  "log": {
    "enabled": true,
    "level": "default",
    "path": "/usr/local/tcp-brutal-manager/test.log"
  },
  "dns": {
    "resolvers": [
      {
        "name": "cloudflare",
        "type": "DoH",
        "address": "cloudflare-dns.com"
      }
    ]
  },
  "rules": [
    {
      "domain": "ipv4.example.com",
      "cidr": {
        "v4": 24
      },
      "rate": 320
    },
    {
      "domain": "ipv6.example.com",
      "cidr": {
        "v6": 64
      },
      "rate": 320
    },
    {
      "domain": "cm-ipv6.example.com",
      "cidr": {
        "v6": 64
      },
      "rate": 500
    },
    {
      "ip": "192.0.2.10",
      "cidr": 32,
      "rate": 200
    },
    {
      "ip": "100.64.0.0",
      "cidr": 10,
      "rate": 200,
      "noroute": true
    }
  ],
  "settings": {
    "dns": {
      "query_interval": "60s",
      "timeout": "5s"
    }
  }
}
```

数值 rate 单位为 Mbps。未给出的 lock 默认 true。100.64.0.0/10 采用经用户确认的 noroute=true；Tailscale 表 52 只有节点 /32 路由，没有精确 /10，故没有给该条使用 interface 策略定位。普通路由由 brutalctl 管理。

## 实测模块与证据

| 模块 | 实测与结果 |
|---|---|
| 安装与初始检查 | 发布摘要核对、包内说明核对、help/list/check、apply dry-run；初始内核无规则。 |
| 基础 Apply | 新增 5 条规则，速率 200/320/500Mbps 分别回读 25000000/40000000/62500000 byte/s；lock=1。再次 Apply 无差异，退出 0。 |
| Tailscale 修正 | 最初无接口 / table 时 brutalctl 创建 ens3 /10 路由；改 noroute 后误建路由移除，table 52 前后相同。 |
| Run 与停机 | 运行约 70 秒，进程持续存活；SIGTERM 退出 0；停止后保留规则，check=0。 |
| DNS System / DoH | System 初始解析通过；Cloudflare cloudflare-dns.com DoH 解析三个域名通过，默认 /dns-query。 |
| DNS 定时与模块分离 | 日志 14:26:32 启动 refresh_dns=true，14:27:33 再次 refresh_dns=true，两轮 exit=0 issues=0；其间非 DNS 核对 refresh_dns=false。 |
| DNS DoT / DoQ | Cloudflare one.one.one.one DoT 与 AdGuard dns.adguard-dns.com DoQ 均 check=0、生成 5 条规则。第一次 DoT 误用 cloudflare-dns.com，连接网站地址超时；改用官方 DoT 主机名后通过，前次不能作为模块缺陷证据。 |
| DNS 选择与 fallback | full 匹配及 domain 数组选择坏路径 Resolver；false 返回 7 且无旧规则删除计划，true 回退首个 Cloudflare 并返回 0；非法 regexp 被跳过，5 条有效规则继续生成，返回 7。 |
| DNS 生命周期 | 仅测试域名使用 localhost DoH 模拟服务、进程专用临时证书；正常 A 创建文档地址规则、AAAA NODATA；HTTP 502 时旧域名规则及 ID 保留，独立规则更新为 201Mbps；NXDOMAIN 时旧规则与来源记录清理。测试证书、服务和规则清除。 |
| 配置监听 | 原子替换配置触发速率 200→201→200 自动更新，日志 refresh_dns=false；无需等 DNS 周期。 |
| 单条故障隔离 | 加入非法 IP 后 run/check 返回 7、1 个错误，其他 5 条有效规则保留；修正后恢复 0。 |
| 路由 table 定位 | 表 20260 文档地址路由写入 congctl lock brutal；lock=false 切为 congctl brutal；MTU 1400 保留。 |
| Route 事件修复 | 手动清除测试路由拥塞算法后，run 事件触发恢复；未触发额外 DNS 刷新。 |
| interface / Link | dummy 接口自动定位表；table+interface 一致匹配成功；两表重复匹配返回局部错误并保护规则；改名触发匹配失败但保留旧规则；删除重建后接口索引 5→6，路由重新定位及修复成功。 |
| 普通属性清理 | 删除策略测试来源，规则清理、congctl 属性移除，Route 本身及 MTU 保留；最终删除测试路由与接口。 |
| 外部规则归属 | 外部文档地址规则同参数复用未认领；不同参数返回冲突、未覆盖；配置来源删除后外部规则仍存在；测试最后明确清理外部条目。发现顺序误报，见下节。 |
| 共享来源 | 相同 Prefix / 参数去重，记录来源数 2；删一来源保留规则、来源数 1；删最后来源清理规则。 |
| Dry-run | run 停止时比较规则值 / ID、受检 proto 233 路由和管理目录文件摘要，apply -n 与 flush -n 前后完全一致。当前无差异 apply -n=0；flush -n=0。 |
| 运行保护 | 第二个 run 退出 1，stderr resource temporarily unavailable；未产生第二个维护进程。 |
| 全局错误重载 | query_interval=0s 被拒绝，旧规则及原 run 存活。 |
| 日志重载失败 | 新日志 /proc/tcpbm-invalid-test/log 无法打开，stderr 报告错误；旧进程与规则保留；恢复配置后 check=0。 |
| 实际 Flush | 用户批准后以 7 条规则（5 业务+2 测试）实测；全部规则及 proto 233 普通路由清空，测试策略属性移除但 Route/MTU 保留，状态记录与属性清空；立即重建业务 5 条，table 52 未变，check=0。 |
| 记录损坏恢复 | 备份记录、保留 initialized、破坏 checksum；dry-run 提示 would_flush=true、退出 3；实际 Apply 仅一次 flush 并重建 5 条，新 checksum 有效、recovery 阶段结束；第二次 Apply 无 flush，规则 ID 不变，check=0。 |
| CLI 最终核对 | check/diff/status/list/version、-h、help apply、help run 正常；前三项无差异、无 issue、无 reorder、无 recovery。 |

### 配置与规则编译矩阵

19/19 用例符合本轮预期：数字 Mbps、2.5Gbps→2500、200000000bps→200、拒绝 2.5Mbps、拒绝 Byte 单位、IPv4 /24 规范化、IPv6 /64 规范化、显式 CIDR 0、重复来源去重、同 Prefix 参数冲突隔离、允许重叠网段、单条字段类型错误隔离、settings 空对象、settings null、空白 Duration、拒绝 0s、拒绝空 rules、拒绝 JSON 语法错误、拒绝重复 Resolver name。对默认值的这组验证证明配置被接受，不是对每一个默认计时器的逐项时间测量。

## 发现的问题

### 1. 外部复用规则参与受管顺序比较

复现：在已有 5 条规则后用 brutalctl 增加 192.0.2.20/32、150Mbps、noroute 外部规则，再在 JSON 加同参数来源。Manager 未认领该条，外部规则参数 / ID 不变，但 Apply 返回 7，issues=[]，would_reorder=true。

预期：只比较受管规则顺序，外部复用规则应排除；无其他差异时应正常返回。

源码定位：internal/manager/manager.go 的 wantedOrder 从全部 desired 生成，actualOrder 与最终 actual 通过 targets 筛选，没有依据 ownership 排除外部复用条目。约在 252–269 及 499–515 行。此缺陷未修复。

区别：添加受管 IPv4 测试规则时，内核把它追加在已有 IPv6 后面，造成真实受管顺序差异；程序返回 7 且不 Flush 重排，符合已确认的可用性优先约定。不能把这类真实顺序退化与外部规则误报混为一谈。

### 2. 发布二进制缺少构建追溯信息

version 返回 commit=dev built=unknown。已安装二进制能够运行，但无法从版本输出定位发布源码提交和构建时间。建议后续构建注入 commit / buildTime 后重新核对。此问题未修复。

## 最终状态与测试边界

| Prefix | Mbps | lock | 路由策略 |
|---|---:|---|---|
| 192.0.2.10/32 | 200 | true | brutalctl 普通路由 |
| 198.51.100.0/24 | 320 | true | brutalctl 普通路由 |
| 100.64.0.0/10 | 200 | true | noroute，Tailscale 节点路由独立保留 |
| 2001:db8:1::/64 | 320 | true | brutalctl 普通路由 |
| 2001:db8:2::/64 | 500 | true | brutalctl 普通路由 |

本轮运行平台仅 amd64；未证明其余已编译架构的真实内核运行行为。未做吞吐压测，内核速率值与连接计数不等同实际吞吐性能证明。未注入磁盘满、进程在每一个持久化阶段崩溃、所有 Netlink 属性种类或 Tailscale 与写入同时竞争等极端场景；也未安装 / 测试 systemd 服务。配置仍在 /tmp，日志保留，run 已停止；业务规则仍有效，不会持续自动更新。

官方协议端点核对来源：[Cloudflare DoT](https://developers.cloudflare.com/1.1.1.1/encryption/dns-over-tls/)；[AdGuard 公共 DNS](https://adguard-dns.io/en/public-dns.html)。


## 后续修复与正式版（2026-10-12）

以上为 Beta 实测时的历史结果。后续已修复外部复用规则顺序误报，并统一六架构构建信息注入；新增回归与正式文档配置解析测试，全量 race/vet 和 Linux CI 通过。正式版本 1.0.0 已合并 main（f115e94a71c1889705606abab9435c5708e130af），GitHub Release v1.0.0 为 Latest、非 Pre-release，六架构包和 SHA256SUMS 已逐项核对摘要，正式配置/CLI/维护文档包含在包内。

本地源码为 GitHub 主线的完整字节/权限匹配副本（34 个 tracked 文件）；当前目录本身仍不是 Git checkout。Skyline 安装文件未在本阶段更新，仍为旧 Beta；运行维护进程已停止，原规则保留。发布截图：RELEASE-1.0.0.jpg。


## 补充：大网段规则与节点路由实测（2026-10-13，脱敏）

Brutal 仅配置一条大网段规则（200Mbps、lock=true、noroute=true），给范围内一个既有节点 /32 路由设置 congctl lock brutal，未新增节点 Brutal 规则。持续下载时 ss 显示节点连接使用 brutal，pacing_rate=200000000bps，末次 delivery_rate 约 199Mbps；大网段规则 members=1，5 秒发送计数增长约 78.1MB，连接 bytes_sent 同步增长。证明本次节点连接使用大网段规则，不代表全部节点或持续吞吐保证。服务器地址、节点 IP、业务域名和连接端口省略。动态路由需事件监听加定时核对，无需拆分 Brutal 规则。
