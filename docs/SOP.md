# 运行维护指南（1.0.0）

本指南描述当前实现。配置字段见 [配置参考](CONFIGURATION.md)，命令和退出码见 [CLI 参考](CLI.md)。设计草案 PLAN.md 为历史资料，发生差异时以正式用户文档及实际实现为准。

## 安装与首次执行

确认 Linux、TCP Brutal 2.0.1 模块已加载、官方 brutalctl 和 iproute2 可执行。安装 Manager 不安装模块、不改变系统默认拥塞算法。

校验下载包 SHA256SUMS，解压，在包目录执行：

```sh
install -m 0755 tcp-brutal-config /usr/local/bin/tcp-brutal-config
install -d -m 0755 /etc/tcp-brutal-config
# 首次配置；若已有配置，先备份并编辑，避免覆盖。
install -m 0644 config.example.json /etc/tcp-brutal-config/config.json
manager_bin=/usr/local/bin/tcp-brutal-config
manager_config=/etc/tcp-brutal-config/config.json
"$manager_bin" version
"$manager_bin" list
"$manager_bin" check -c "$manager_config"
"$manager_bin" apply -c "$manager_config" -n
```

先编辑示例目标/速率，检查 issues 和路由策略后再执行 apply。若采用自定义安装目录，只改变变量及服务的 ExecStart；默认配置、状态和日志位置不随程序改名改变。

## 一次性应用与持续运行

Apply 在修改锁外查询 DNS，锁内重新读取规则/路由/记录并规划；写前重读路由，只修改拥塞算法及相关锁位，写后验证。局部失败只保护对应可识别来源，独立规则继续。正常 Apply 不 Flush，不承诺全事务回滚。规则改动完成后再次 check；没有差异应为 0。

Run 可前台运行，SIGINT/SIGTERM 保留现有规则：

```sh
"$manager_bin" run -c "$manager_config"
```

| 触发 | 处理 |
|---|---|
| DNS query_interval | 实时查询，变化后更新规则；默认 60s。 |
| 配置目录事件 | 去抖后加载；内容重扫作为兜底。 |
| Route/Link Netlink 事件 | 重新定位/修复策略路由，使用本进程 DNS 结果。 |
| route resync_interval | 路由核对兜底。 |
| brutal poll_interval | 读取受管规则并修复差异，默认 30s。 |

全局非法重载、不可读配置或新 Logger 初始化失败时保留旧配置及 Logger。业务单条错误隔离。只有一个 Run；一次性写操作另受修改锁保护。运行日志中的 refresh_dns=false 表示本轮使用进程内解析结果，true 表示刷新 DNS。

## systemd

包内提供模板，默认程序位置 /usr/local/bin/tcp-brutal-config。先核对 ExecStart 的真实程序和配置路径，再安装：

```sh
install -m 0644 tcp-brutal-config.service /etc/systemd/system/tcp-brutal-config.service
systemctl daemon-reload
systemctl enable --now tcp-brutal-config.service
systemctl status tcp-brutal-config.service
```

使用 Tailscale 策略路由时可添加 After=tailscaled.service 的 drop-in，但接口暂不可用仍需核对日志。Restart=on-failure；Run 的正常信号退出保留规则。systemd 模板未在本轮 live 测试中安装，不把该模板列为已实测部署保障。

## Tailscale 与策略路由

Tailscale 常在 table 52 建立节点 /32 路由。100.64.0.0/10 不等于某个节点的 /32：只有 /32 时，配置 /10+interface=tailscale0 会因没有精确匹配而跳过。

- 希望 /10 Brutal 规则覆盖网段、保留 Tailscale 节点路由：配置 noroute=true，不指定 table/interface。
- 希望 Go 维护某节点路由属性：配置该节点 IP、cidr=32，再填写 interface=tailscale0，可自动确定唯一表；也可同时填 table=52。
- 不填写 table/interface 且 noroute=false 时，brutalctl 可能按普通默认出口创建路由。检查计划与实际路由，不从地址名称推断出口。

策略路由必须已经存在；程序不创建整个 Table、Peer Route 或 ip rule。允许覆盖已有其他拥塞算法，清理不恢复旧算法。brutalctl 的 noroute/del 若会删外部 proto 233 路由，则隔离该规则。Route 的其他字段与 metrics 锁位保留，但读改写不是原子操作，外部竞争会导致暂缓或验证失败。

## Flush 与清理

要保持清空先停止 Run 或服务，再预览、执行 Flush：

```sh
systemctl stop tcp-brutal-config.service # 仅在已安装服务时使用
"$manager_bin" flush -n
"$manager_bin" flush
"$manager_bin" list
```

Flush 委托 brutalctl 清空全部规则及其普通路由清理范围，包含外部规则。Go 记录的属性按每个已写入值匹配移除，不匹配的字段保留；Route/网关/MTU 保留，不恢复旧值。仅添加锁位时不删除原有算法。局部清理失败继续其他项，并保留失败记录重试。再次运行维护会重新应用配置。

直接 brutalctl flush 不执行 Manager 的策略属性清理。管理记录损坏时 Manager flush 会拒绝；按下面的 Apply 恢复流程处理，不用删标记绕过检查。

## 管理记录异常恢复

状态目录 /var/lib/tcp-brutal-config 中 state.json 包含校验和、已成功规则与来源、Go 属性、待完成意图/恢复阶段；initialized 用于区分曾经初始化与首次安装。

| 状态 | 行为 |
|---|---|
| 无记录且无标记 | 首次安装，不自动清空未知规则。 |
| 标记存在，记录丢失或校验损坏 | Apply 全局恢复：Flush 全部规则，清理所有表的 brutal 属性及对应锁位，再按当前有效配置重建。 |
| 标记和记录都丢失 | 无法识别历史，按首次安装；需要清空由用户显式 Flush。 |

恢复前停止其他 Manager 写操作，备份状态和配置，核对当前命名空间所有表的 brutal 路由范围；check 或 apply -n 可显示 would_flush。重建不是旧记录回滚，DNS 故障来源的旧状态不保证恢复。恢复阶段持久化，重试不重复已经完成的全量 Flush；旧清理任务执行前检查新有效需求。

保持配置文件路径稳定，来源包含配置路径、IP/域名和重复出现序号。无法识别旧来源的非法编辑不会保护所有旧规则。不要把记录当成 DNS 缓存随意删除，也不要把旧备份直接覆盖到变化后的系统。

## 排错与边界

check=3 先看差异，7 先看 issues 与 would_reorder。普通配置/模块/权限故障看 stderr、文件日志和真实内核状态。规则部分完成时不假设已回滚；修正问题后重新 check/dry-run/apply。

真实受管顺序不一致而无法无损重排时，Apply 保留运行状态并返回 7；不自动 Flush。外部未认领的复用规则不参与受管顺序比较。

当前实测为 Linux amd64、TCP Brutal 2.0.1，覆盖公共 DNS 四种解析方式及规则/路由/监听/日志/恢复主要行为。其他架构仅交叉编译；吞吐、磁盘满、所有崩溃窗口、外部同时重建路由等未穷尽验证。
