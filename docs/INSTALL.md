# 在线安装

根目录 install.sh 从 GitHub Releases 下载，默认最新稳定版；支持 install、update、check、--beta 和 --version。当前稳定版为 1.0.1。架构范围为 Linux amd64、arm64、386、armv5、armv6、armv7。

## 私有仓库

仓库目前为私有，匿名 curl 不能读取安装脚本或发布附件。下载和安装都需要具备本仓库读取权限；不要把令牌写进命令参数、脚本或配置。

在目标 Linux 主机用已登录的 GitHub CLI 取脚本：

```bash
gh api --method GET 'repos/XHQ6666/TCP-Brutal-Manager/contents/install.sh?ref=main' \
  -H 'Accept: application/vnd.github.raw+json' > install.sh
bash install.sh --help
```

安装器本身不依赖 gh：有 GH_TOKEN/GITHUB_TOKEN 时使用环境变量，否则可复用已登录 gh 的令牌。使用 sudo 时需将认证传递给 root；可以在本地 shell 安全读取环境变量，不将令牌值写进历史：

```bash
read -rsp 'GitHub token: ' GH_TOKEN; echo
export GH_TOKEN
sudo --preserve-env=GH_TOKEN bash install.sh install --config /path/to/config.json
unset GH_TOKEN
```

已经以 root 登录 gh 的用户也可直接执行 bash install.sh install。仓库若未来公开，可匿名 HTTPS 下载；本脚本不会改变仓库可见性。

## 安装、升级和查询

首次安装必须提供自己的配置，不会用示例规则启动：

```bash
sudo --preserve-env=GH_TOKEN bash install.sh install --config /path/to/config.json
```

升级保留已有配置和管理记录，禁止 --config 覆盖：

```bash
sudo --preserve-env=GH_TOKEN bash install.sh update
bash install.sh check
sudo --preserve-env=GH_TOKEN bash install.sh update --version 1.0.1
sudo --preserve-env=GH_TOKEN bash install.sh update --beta
```

默认稳定通道不选预发布。--beta 选择最新兼容发布（可含正式版），--version 明确安装版本，不与 --beta 同用。低于 1.0.1 的旧名称包不由此脚本安装。check 不需要 root、不修改配置或服务。安装与升级需要 root、systemd、Python 3、flock、已加载的 TCP Brutal 模块及 brutalctl；不会自动安装模块或更改默认拥塞算法。

## 本地包

将发布包与同一版本的 SHA256SUMS 放在同一目录：

```bash
sudo bash install.sh --archive /path/tcp-brutal-manager-1.0.1-linux-amd64.tar.gz
```

会识别包名中的版本并校验目标架构。首次离线安装同时加 --config。--no-start 安装并预检后保持停止；旧活动目录暂保留，需手动启动核对后清理。

## 行为与边界

- 下载重试、HTTPS、SHA256 和程序版本核对完成后才安装；跨域附件重定向不携带 GitHub 认证头。
- 安装到 /usr/local/tcp-brutal-manager，服务为 tcp-brutal-manager.service，配置 /etc/tcp-brutal-manager/config.json，状态 /var/lib/tcp-brutal-manager。
- 配置和状态数据备份在状态目录 deploy-backups；不备份旧二进制。新旧服务不会同时启动。
- 旧名称迁移保留业务规则、DNS 和 settings；旧默认日志路径改为新名称，来源路径同步迁移并重算校验。新旧状态同时存在时拒绝猜测归属。
- 状态损坏、恢复未完成、预检要求 Flush 或有局部错误时拒绝部署。预检失败后服务可能保持停止，需要核对原因再启动；不会自动回滚已发生的内核运行状态。
- 启动及 check 一致性通过后，才删除旧名称程序、服务和旧活动目录。其他安装目录不擅自清理。
- 首次启动会应用配置并修改 Brutal 规则及路由属性；安装器不执行 Flush。

参考官方项目的安装接口与行为：[Xray-install](https://github.com/XTLS/Xray-install)、[sing-box 安装说明](https://sing-box.sagernet.org/installation/package-manager/)。本项目只提供已有六种架构的 tar.gz 包，不声称支持 deb/rpm 或 OpenRC。
