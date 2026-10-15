# 一键安装

在目标 Linux 主机以 root 运行 install.sh，无参数就是安装或更新最新正式版。

## 一条命令

在目标 Linux 主机的 root 终端执行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/XHQ6666/TCP-Brutal-Manager/main/install.sh)
```

再次运行同一命令即可升级，保留已有配置。无需 GitHub CLI 或令牌。

指定版本或查询版本：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/XHQ6666/TCP-Brutal-Manager/main/install.sh) --version 1.0.1
bash <(curl -fsSL https://raw.githubusercontent.com/XHQ6666/TCP-Brutal-Manager/main/install.sh) check
```

也可先下载再查看和执行：

```bash
curl -fsSL https://raw.githubusercontent.com/XHQ6666/TCP-Brutal-Manager/main/install.sh -o install.sh
bash install.sh
```

GitHub API 限流时可以选择提供 GH_TOKEN/GITHUB_TOKEN；不要把令牌放进 URL 或命令参数。安装器不会要求为公开仓库登录账号。

## 默认流程

1. 检测 Linux、systemd 和 CPU 架构，按需准备 Python 3、util-linux、CA 证书。
2. 下载最新版与校验文件，核对 SHA256 和程序版本。
3. 安装程序和 systemd 服务。
4. 已有配置：原样保留业务规则、DNS 和 settings，验证后启动服务。
5. 首次没有配置：只放置 config.example.json，不启动服务、不应用示例规则。

首次安装完成后，填写自己的配置再启动：

```bash
cp /etc/tcp-brutal-manager/config.example.json /etc/tcp-brutal-manager/config.json
# 编辑 config.json，填写自己的目标和速率
systemctl enable --now tcp-brutal-manager
```

TCP Brutal 内核模块和 brutalctl 由[官方项目](https://github.com/HyNetworks/tcp-brutal)安装；本脚本不自动安装或切换内核模块。已有配置准备启动时会检查它们是否可用。新装无配置时可先安装 Manager，后准备内核模块。

## 可选用法

```bash
bash install.sh update                  # 与无参数执行相同，保留配置升级
bash install.sh --version 1.0.1         # 指定版本
bash install.sh --beta                  # 最新兼容版本，包括预发布
bash install.sh check                   # 只查版本，不修改系统
bash install.sh --config /path/config.json  # 首次直接导入配置并启动
bash install.sh --no-start              # 安装但不启动
bash install.sh --archive /path/tcp-brutal-manager-1.0.1-linux-amd64.tar.gz
```

本地包旁边需有同一版本的 SHA256SUMS。默认不选预发布；--beta 不与 --version/--archive 同用。只支持已发布的 amd64、arm64、386、armv5、armv6、armv7，不声称支持 OpenRC、deb/rpm 或其他 CPU 架构。

## 路径和升级

程序 /usr/local/tcp-brutal-manager/tcp-brutal-manager，通过 /usr/local/bin/tcp-brutal-manager 提供命令入口；服务 tcp-brutal-manager.service；配置 /etc/tcp-brutal-manager/config.json；状态 /var/lib/tcp-brutal-manager。

旧名称迁移会同步管理来源路径和校验，旧默认日志路径改为新名称，不拆分 Brutal 规则，不 Flush。配置和状态数据快照放在状态目录 deploy-backups，成功后删除旧名称程序、服务及旧活动目录，不保留旧二进制。新旧状态同时存在、状态损坏、恢复未完成或预检发现错误/Flush 计划时停止，不猜测归属。失败后服务可能保持停止，应核对原因再启动；不会自动回滚已发生的内核状态。

## 参考

入口与默认安装体验参考 [3x-ui](https://github.com/MHSanaei/3x-ui/blob/master/install.sh) 和 [Hysteria](https://v2.hysteria.network/docs/getting-started/Installation/)。主流脚本也可能很长；这里简化的是用户操作，保留必要的下载校验、配置保护和迁移处理。
