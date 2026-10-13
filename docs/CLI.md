# CLI 参考（1.0.0）

文档使用实际文件名变量，Help 的程序名来自调用路径：

```sh
manager_bin=/usr/local/bin/tcp-brutal-config
manager_config=/etc/tcp-brutal-config/config.json
```

## 命令

| 命令 | 行为 |
|---|---|
| help / -h / --help | 帮助；help apply 或 help run 可指定命令。 |
| version / -v / --version | 输出版本、源码提交号和 UTC 构建时间。Help/Version 不加载配置或要求模块。 |
| list | 只读内核规则，输出 Prefix、byte/s、lock 和可用字段，不需要配置。 |
| check | 全局/逐条校验、实时 DNS 解析和差异计划。 |
| diff | 当前实现与 check 使用相同计划与退出语义。 |
| status | 当前实现同样输出实时计划；不是额外健康仪表盘或历史摘要。 |
| apply | 执行计划，逐条隔离局部失败，回读并保存已成功状态。正常过程不全量 Flush。 |
| run | 启动立即维护，监听配置及 Route/Link，独立定时 DNS 解析与规则核对；单实例。 |
| flush | 清空全部 Brutal 规则，清理匹配的 Go 路由属性；不是只清空自己的规则。 |

-c/--config 指定配置，支持 --config=path；默认 /etc/tcp-brutal-config/config.json。-n/--dry-run 仅适用于 apply/flush，禁止用于 run。Dry-run 不写规则、路由或管理记录；文件日志仍遵守配置。纯 Flush 使用默认维护参数和状态目录，不需要业务 rules。

## 示例

```sh
"$manager_bin" check -c "$manager_config"
"$manager_bin" diff -c "$manager_config"
"$manager_bin" apply -c "$manager_config" -n
"$manager_bin" apply -c "$manager_config"
"$manager_bin" run -c "$manager_config"
# 停止 Run 后再清空：
"$manager_bin" flush -n
"$manager_bin" flush
"$manager_bin" list
```

实时计划包含 desired、would_add、would_update、would_remove、would_repair_route、would_cleanup_attributes、issues、would_reorder、would_flush。issues 的 index 从 0 开始，汇总/运行阶段错误可用 -1。Apply 输出的是操作计划；命令完成后用 list/check 回读核对，不把 would_add 字段理解成最终结果列表。

## 退出码

| 码 | 含义 |
|---|---|
| 0 | 成功或一致；Run 正常信号停止也为 0。 |
| 1 | 运行时全局失败、锁占用等。 |
| 2 | 全局配置/CLI 错误。 |
| 3 | check/diff/status/apply dry-run 发现普通差异（包括待处理的顺序差异）。 |
| 4 | 全局验证错误保留码。 |
| 5 | 权限错误分类保留码。 |
| 6 | 内核/依赖读取错误或不支持的平台。 |
| 7 | 局部失败、冲突/跳过或 Apply 无法无损修复顺序后的退化结果。 |

优先处理全局错误，再处理局部 issues，最后看普通差异。运行时并非每个底层错误都能精确归入专用码，需同时读取 stderr/issues。Flush dry-run 当前正常返回 0；Run 正常退出 0 不证明之前每轮维护都健康，应核对日志及 check。

Apply/check/diff/status 每次重新解析 DNS。Run 的配置/路由/规则核对事件使用本进程最近成功结果；DNS 定时器和相关域名/Resolver 配置变化才刷新，重启仍重新解析。


### CIDR 路由维护输出

`route_details` 展示业务 Prefix、实际 Route、route_strict 和计划动作；`route_summary` 中 matched 是解析重叠策略后的路由数量。check/diff/status 的 succeeded/failed 均为 0（未执行），Apply/Run 则记录完成维护的路由与失败数。无候选或路由局部失败返回 7，其他规则继续。
