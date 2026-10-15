#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
VERSION=''
BETA=0
ACTION=install
ACTION_SEEN=0
REPO=XHQ6666/TCP-Brutal-Manager
ARCHIVE=''
INPUT_CONFIG=''
START=1
PREFIX=/usr/local/tcp-brutal-manager
CONFIG=/etc/tcp-brutal-manager/config.json
STATE=/var/lib/tcp-brutal-manager
SERVICE=tcp-brutal-manager.service
usage() {
  cat <<'HELP'
TCP Brutal Manager 安装脚本

直接运行：安装最新正式版；已有配置自动保留。
首次没有配置：安装程序、服务和配置模板，不启动、不修改网络规则。

  bash install.sh                  安装 / 更新
  bash install.sh --version 1.0.1  指定版本
  bash install.sh --beta           最新版（含预发布）
  bash install.sh check            查询版本
  bash install.sh --help           帮助

可选：--config FILE 首次导入配置，--archive FILE 本地包，--no-start 不启动。
自动准备基础依赖；不自动安装 TCP Brutal 内核模块。
私有仓库支持 GH_TOKEN/GITHUB_TOKEN，或复用已登录 gh；不需要预装 gh。
升级保留配置和管理记录；不执行 Flush。
HELP
}
while (($#)); do
  case "$1" in
    install|update|check) ((ACTION_SEEN==0)) || { echo '只能指定一个操作' >&2; exit 2; }; ACTION=$1; ACTION_SEEN=1; shift;;
    --beta) BETA=1; shift;;
    --version|--archive|--config) (($#>=2)) || { echo "缺少参数值：$1" >&2; exit 2; }; case "$1" in --version) VERSION=${2#v};; --archive) ARCHIVE=$2;; --config) INPUT_CONFIG=$2;; esac; shift 2;;
    --no-start) START=0; shift;;
    -h|--help) usage; exit 0;;
    *) echo "未知参数：$1" >&2; exit 2;;
  esac
done
[[ -z $VERSION || $VERSION =~ ^[A-Za-z0-9][A-Za-z0-9._+-]*$ ]] || { echo '版本格式错误' >&2; exit 2; }
((BETA==0)) || [[ -z $VERSION && -z $ARCHIVE ]] || { echo '--beta 不能与 --version/--archive 同用' >&2; exit 2; }
[[ $ACTION == check || ( $(uname -s) == Linux && $EUID == 0 ) ]] || { echo '请在目标 Linux 主机以 root 执行。' >&2; exit 1; }
if ! command -v python3 >/dev/null || { [[ $ACTION != check ]] && ! command -v flock >/dev/null; }; then
  [[ $ACTION != check ]] || { echo '版本查询需要 python3。' >&2; exit 1; }
  echo '正在准备基础依赖…'
  if command -v apt-get >/dev/null; then
    apt-get update; DEBIAN_FRONTEND=noninteractive apt-get install -y python3 util-linux ca-certificates
  elif command -v dnf >/dev/null; then
    dnf install -y python3 util-linux ca-certificates
  elif command -v yum >/dev/null; then
    yum install -y python3 util-linux ca-certificates
  elif command -v zypper >/dev/null; then
    zypper --non-interactive install python3 util-linux ca-certificates
  elif command -v pacman >/dev/null; then
    pacman -S --needed --noconfirm python util-linux ca-certificates
  else
    echo '请先安装 python3、util-linux、ca-certificates。' >&2; exit 1
  fi
fi
if [[ $ACTION != check ]]; then
 for cmd in systemctl install flock mktemp; do command -v "$cmd" >/dev/null || { echo "缺少依赖：$cmd" >&2; exit 1; }; done
 [[ -d /run/systemd/system ]] || { echo '需要 systemd 系统。' >&2; exit 1; }
fi
if [[ -n $ARCHIVE && -z $VERSION ]]; then
  VERSION=$(python3 - "$ARCHIVE" <<'INFER_PY'
import pathlib,re,sys
m=re.fullmatch(r'tcp-brutal-manager-(.+)-linux-(?:amd64|arm64|386|armv[567])\.tar\.gz',pathlib.Path(sys.argv[1]).name)
if not m:raise SystemExit('无法从本地发布包识别版本，请指定 --version')
print(m[1])
INFER_PY
)
fi
case $(uname -m) in
  x86_64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; i?86) ARCH=386;;
  armv5*) ARCH=armv5;; armv6*) ARCH=armv6;; armv7*|armv8l) ARCH=armv7;;
  *) echo '此 CPU 架构没有发布包。' >&2; exit 1;;
esac
if [[ $ACTION != check ]]; then
 exec 9>/run/tcp-brutal-manager-deploy.lock
 flock -n 9 || { echo '另一个部署正在执行。' >&2; exit 1; }
fi
WORK=$(mktemp -d /tmp/tcpbm-deploy.XXXXXXXX)
trap 'rm -rf -- "$WORK"' EXIT
trap 'echo "部署未完成；请核对服务与配置。不会自动 Flush 或回滚运行状态。" >&2' ERR
if [[ -z $ARCHIVE ]]; then
  python3 - "$REPO" "$VERSION" "$BETA" "$ARCH" "$WORK" "$ACTION" <<'DOWNLOAD_PY'
import os,sys,json,pathlib,urllib.request,urllib.parse,urllib.error,shutil,time,subprocess,re
repo,version,beta,arch,dest,action=sys.argv[1:]
token=os.environ.get('GH_TOKEN') or os.environ.get('GITHUB_TOKEN')
if not token and shutil.which('gh'):
    auth=subprocess.run(['gh','auth','token','--hostname','github.com'],capture_output=True,text=True)
    if auth.returncode==0:token=auth.stdout.strip()
class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,req,fp,code,msg,headers,newurl):
        if urllib.parse.urlparse(newurl).scheme!='https':raise ValueError('拒绝非 HTTPS 重定向')
        result=super().redirect_request(req,fp,code,msg,headers,newurl)
        if result is not None and urllib.parse.urlparse(req.full_url).netloc!=urllib.parse.urlparse(newurl).netloc:result.remove_header('Authorization')
        return result
opener=urllib.request.build_opener(SafeRedirect())
def request(url,accept):
    parsed=urllib.parse.urlparse(url)
    if parsed.scheme!='https' or parsed.hostname!='api.github.com':raise ValueError('下载 API 地址不合法')
    headers={'Accept':accept,'User-Agent':'TCP-Brutal-Manager-installer'}
    if token:headers['Authorization']='Bearer '+token
    for attempt in range(3):
        try:return opener.open(urllib.request.Request(url,headers=headers),timeout=60)
        except urllib.error.HTTPError as e:
            if e.code not in (429,500,502,503,504) or attempt==2:raise
        except urllib.error.URLError:
            if attempt==2:raise
        time.sleep(2*(attempt+1))
def get(path):
    with request('https://api.github.com/repos/'+repo+path,'application/vnd.github+json') as res:return json.load(res)
def compatible(release):
    tag=release.get('tag_name','');m=re.fullmatch(r'v?(\d+)\.(\d+)\.(\d+)(?:[-+][A-Za-z0-9._+-]+)?',tag)
    return not release.get('draft') and m and tuple(map(int,m.groups()))>=(1,0,1)
try:
    if version:release=get('/releases/tags/'+urllib.parse.quote('v'+version.lower(),safe=''))
    elif beta=='1':
        releases=get('/releases?per_page=100');release=next((r for r in releases if compatible(r)),None)
        if release is None:raise ValueError('无兼容发布版本')
    else:release=get('/releases/latest')
    if not compatible(release):raise SystemExit('目标版本使用旧命名或不兼容；当前请指定 --version 1.0.1 或 --beta')
    selected=release['tag_name'][1:] if release['tag_name'].startswith('v') else release['tag_name'];version=selected
    name='tcp-brutal-manager-'+version.lower()+'-linux-'+arch+'.tar.gz'
    assets={a['name']:a for a in release['assets']}
    for filename in (name,'SHA256SUMS'):
        if filename not in assets:raise ValueError('发布附件不完整')
    pathlib.Path(dest,'selected-version').write_text(version)
    pathlib.Path(dest,'selected-package').write_text(name)
    print('目标版本：'+version+(' (Pre-Release)' if release.get('prerelease') else ''))
    if action!='check':
        for filename in (name,'SHA256SUMS'):
            for attempt in range(3):
                try:
                    with request(assets[filename]['url'],'application/octet-stream') as res, open(pathlib.Path(dest)/filename,'wb') as out:shutil.copyfileobj(res,out)
                    break
                except (urllib.error.URLError,OSError):
                    if attempt==2:raise
                    time.sleep(2*(attempt+1))
except urllib.error.HTTPError as e:raise SystemExit('GitHub 请求失败（HTTP '+str(e.code)+'）；私有仓库请核对 GH_TOKEN 和读取权限')
except (urllib.error.URLError,ValueError,KeyError,OSError):raise SystemExit('版本查询或下载失败，请核对网络、版本和附件')
DOWNLOAD_PY
  VERSION=$(cat "$WORK/selected-version")
  FILE=$(cat "$WORK/selected-package")
  ARCHIVE=$WORK/$FILE
fi
if [[ $ACTION == check ]]; then
  if [[ -n $ARCHIVE && ! -f $WORK/selected-version ]]; then echo "目标版本：$VERSION（本地包）"; fi
  if [[ -x $PREFIX/tcp-brutal-manager ]]; then "$PREFIX/tcp-brutal-manager" version; else echo '本机未安装新名称的程序。'; fi
  exit 0
fi
[[ -f $ARCHIVE && -f $(dirname -- "$ARCHIVE")/SHA256SUMS ]] || { echo '发布包或 SHA256SUMS 不存在。' >&2; exit 1; }
python3 - "$ARCHIVE" "$WORK/binary" "$VERSION" "$ARCH" <<'PY'
import hashlib,pathlib,sys,tarfile
archive,dest,version,arch=map(str,sys.argv[1:]);p=pathlib.Path(archive)
expected_name=f'tcp-brutal-manager-{version.lower()}-linux-{arch}.tar.gz'
if p.name!=expected_name:raise SystemExit('发布包名称与版本/架构不符')
lines=(p.parent/'SHA256SUMS').read_text().splitlines()
matches=[x.split()[0] for x in lines if len(x.split())==2 and x.split()[1].lstrip('*')==p.name]
if len(matches)!=1 or hashlib.sha256(p.read_bytes()).hexdigest()!=matches[0]:raise SystemExit('SHA256 校验失败')
with tarfile.open(p) as tar:
    member=tar.getmember(p.name[:-7]+'/tcp-brutal-manager')
    if not member.isfile() or member.size>100_000_000:raise SystemExit('发布包二进制异常')
    pathlib.Path(dest).write_bytes(tar.extractfile(member).read())
pathlib.Path(dest).chmod(0o755)
PY
"$WORK/binary" version > "$WORK/version.txt"
python3 - "$WORK/version.txt" "$VERSION" <<'PY'
import pathlib,sys
v=pathlib.Path(sys.argv[1]).read_text().strip()
if not v.startswith(sys.argv[2]+' commit='):raise SystemExit('程序版本不符')
print(v)
PY
CONFIG_READY=1
if [[ ! -f $CONFIG && ! -f /etc/tcp-brutal-config/config.json && -z $INPUT_CONFIG ]]; then
  if [[ -e $STATE/initialized || -e $STATE/state.json || -e /var/lib/tcp-brutal-config/initialized || -e /var/lib/tcp-brutal-config/state.json ]]; then
    echo '存在管理记录但配置缺失，请恢复配置后重试。' >&2; exit 1
  fi
  CONFIG_READY=0
  START=0
fi
if ((CONFIG_READY)); then
 command -v brutalctl >/dev/null || { echo '请先安装官方 TCP Brutal（含 brutalctl）：https://github.com/HyNetworks/tcp-brutal' >&2; exit 1; }
 [[ -r /proc/net/tcp_brutal/rules ]] || { echo '请先加载 TCP Brutal 内核模块（modprobe brutal）。' >&2; exit 1; }
# Validate source configuration and state before stopping services.
python3 - "$INPUT_CONFIG" <<'PY'
import pathlib,json,hashlib,sys
new=pathlib.Path('/etc/tcp-brutal-manager/config.json');old=pathlib.Path('/etc/tcp-brutal-config/config.json')
provided=pathlib.Path(sys.argv[1]) if sys.argv[1] else None
if provided and (new.exists() or old.exists()):raise SystemExit('已有配置，拒绝 --config 覆盖；请直接升级')
source=new if new.exists() else old if old.exists() else provided
if source is None or not source.is_file():raise SystemExit('首次安装必须通过 --config 提供配置')
json.loads(source.read_text())
a=pathlib.Path('/var/lib/tcp-brutal-manager');b=pathlib.Path('/var/lib/tcp-brutal-config')
if (a/'initialized').exists() and (b/'initialized').exists():raise SystemExit('新旧状态同时存在，请先人工核对')
if not new.exists() and not old.exists() and ((a/'initialized').exists() or (b/'initialized').exists()):raise SystemExit('已有管理记录但原配置缺失，请先核对来源路径')
if new.exists() and not (a/'state.json').exists() and (b/'initialized').exists():raise SystemExit('配置与状态混用新旧目录，请先人工核对')
for state in [a,b]:
    marker=(state/'initialized').exists();p=state/'state.json'
    if marker and not p.exists():raise SystemExit('管理记录丢失，拒绝部署以避免自动恢复清空')
    if p.exists():
        env=json.loads(p.read_text());data=env['data'];compact=json.dumps(data,separators=(',',':'),ensure_ascii=False).encode()
        if hashlib.sha256(compact).hexdigest()!=env['checksum'] or data['version'] not in [1,2] or data.get('recovery'):raise SystemExit('状态校验/恢复阶段异常，拒绝部署')
PY
systemctl stop tcp-brutal-manager.service 2>/dev/null || true
systemctl stop tcp-brutal-config.service 2>/dev/null || true
# Refuse any surviving manager process, including manually started Run instances.
python3 - <<'PY'
import pathlib
for p in pathlib.Path('/proc').glob('[0-9]*/cmdline'):
    try:args=p.read_bytes().split(b'\0')
    except OSError:continue
    if args and pathlib.Path(args[0].decode(errors='replace')).name in ('tcp-brutal-manager','tcp-brutal-config') and b'run' in args:raise SystemExit('仍有运行中的 Manager；请停止后重试')
PY
python3 - "$INPUT_CONFIG" <<'PY'
import pathlib,json,hashlib,sys,shutil,os,datetime
new=pathlib.Path('/etc/tcp-brutal-manager/config.json');old=pathlib.Path('/etc/tcp-brutal-config/config.json')
state=pathlib.Path('/var/lib/tcp-brutal-manager');legacy=pathlib.Path('/var/lib/tcp-brutal-config')
source=new if new.exists() else old if old.exists() else pathlib.Path(sys.argv[1])
config=json.loads(source.read_text());original_path=str(source.resolve())
if not state.exists() and legacy.exists():shutil.copytree(legacy,state)
state.mkdir(mode=0o700,parents=True,exist_ok=True)
backup=state/'deploy-backups'/datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%S%fZ');backup.mkdir(mode=0o700,parents=True)
shutil.copy2(source,backup/'config.json')
record=state/'state.json'
if record.exists():shutil.copy2(record,backup/'state.json')
if config.get('log',{}).get('path')=='/var/log/tcp-brutal-config.log':config['log']['path']='/var/log/tcp-brutal-manager.log'
new.parent.mkdir(mode=0o755,parents=True,exist_ok=True)
if source!=new:
    tmp=new.with_suffix('.next');tmp.write_text(json.dumps(config,ensure_ascii=False,indent=2)+'\n');tmp.chmod(0o600);os.replace(tmp,new)
elif json.loads(new.read_text())!=config:
    tmp=new.with_suffix('.next');tmp.write_text(json.dumps(config,ensure_ascii=False,indent=2)+'\n');tmp.chmod(new.stat().st_mode&0o777);os.replace(tmp,new)
if record.exists() and original_path!=str(new):
    env=json.loads(record.read_text())
    def rewrite(v):
        if isinstance(v,str) and v.startswith(original_path+'|'):return str(new)+v[len(original_path):]
        if isinstance(v,list):return [rewrite(x) for x in v]
        if isinstance(v,dict):return {k:rewrite(x) for k,x in v.items()}
        return v
    data=rewrite(env['data']);data['generation']=hashlib.sha256(new.read_bytes()).hexdigest()
    compact=json.dumps(data,separators=(',',':'),ensure_ascii=False).encode()
    tmp=record.with_suffix('.next');tmp.write_text(json.dumps({'data':data,'checksum':hashlib.sha256(compact).hexdigest()},ensure_ascii=False,indent=2)+'\n');tmp.chmod(0o600);os.replace(tmp,record)
PY
set +e
"$WORK/binary" check -c "$CONFIG" > "$WORK/plan.json" 2> "$WORK/check.err"
CHECK_CODE=$?
set -e
python3 - "$WORK/plan.json" "$CHECK_CODE" <<'PY'
import json,pathlib,sys
if int(sys.argv[2]) not in (0,3):raise SystemExit('配置预检未通过；配置/状态备份已保留，服务未启动')
p=json.loads(pathlib.Path(sys.argv[1]).read_text())
if p.get('would_flush') or p.get('issues'):raise SystemExit('拒绝会 Flush 或存在错误的部署')
PY
fi
install -d -m 0755 "$PREFIX"
install -m 0755 "$WORK/binary" "$PREFIX/tcp-brutal-manager.next"
mv -f "$PREFIX/tcp-brutal-manager.next" "$PREFIX/tcp-brutal-manager"
if [[ ! -e /usr/local/bin/tcp-brutal-manager || -L /usr/local/bin/tcp-brutal-manager ]]; then
  ln -sfn "$PREFIX/tcp-brutal-manager" /usr/local/bin/tcp-brutal-manager
fi
cat > /etc/systemd/system/tcp-brutal-manager.service <<'UNIT'
[Unit]
Description=TCP Brutal Manager
Wants=network-online.target
After=network-online.target tailscaled.service
[Service]
Type=simple
ExecStart=/usr/local/tcp-brutal-manager/tcp-brutal-manager run -c /etc/tcp-brutal-manager/config.json
Restart=on-failure
RestartSec=5s
[Install]
WantedBy=multi-user.target
UNIT
chmod 0644 /etc/systemd/system/tcp-brutal-manager.service
systemctl daemon-reload
if ((CONFIG_READY==0)); then
  install -d -m 0755 /etc/tcp-brutal-manager
  if [[ ! -e /etc/tcp-brutal-manager/config.example.json ]]; then
    cat > /etc/tcp-brutal-manager/config.example.json <<'EXAMPLE'
{
  "rules": [
    { "ip": "192.0.2.1", "cidr": 32, "rate": 200 }
  ],
  "settings": {}
}
EXAMPLE
    chmod 0600 /etc/tcp-brutal-manager/config.example.json
  fi
  echo '安装完成，示例规则未应用。'
  echo '1. 将 config.example.json 复制为 /etc/tcp-brutal-manager/config.json 并填写自己的规则。'
  echo '2. 安装并加载官方 TCP Brutal 模块（含 brutalctl）。'
  echo '3. 执行：systemctl enable --now tcp-brutal-manager'
  exit 0
fi
if ((START)); then
  systemctl enable --now "$SERVICE"
  READY=0
  for attempt in {1..15}; do
    sleep 2
    systemctl is-active --quiet "$SERVICE" || break
    if "$PREFIX/tcp-brutal-manager" check > "$WORK/post.json" 2>/dev/null; then READY=1; break; fi
  done
  ((READY)) || { echo '服务核对未通过，保留旧活动目录供核对；不自动回滚状态。' >&2; exit 1; }
  python3 - "$WORK/post.json" <<'PY'
import json,sys,pathlib
p=json.loads(pathlib.Path(sys.argv[1]).read_text())
if p.get('issues') or p.get('would_flush'):raise SystemExit('部署后状态异常')
print(f"部署成功：{len(p['desired'])} 条规则，{p['route_summary']['matched']} 条匹配路由，无错误。")
PY
else
  echo '已安装但未启动。旧名称活动目录暂保留，启动核对后再清理。'
  exit 0
fi
systemctl disable tcp-brutal-config.service 2>/dev/null || true
rm -f /etc/systemd/system/tcp-brutal-config.service /usr/local/bin/tcp-brutal-config
python3 - <<'PY'
import pathlib,shutil
base=pathlib.Path('/usr/local/tcp-brutal-manager')
for p in base.rglob('tcp-brutal-config*'):
    if p.is_file() or p.is_symlink():p.unlink()
for p in ['/etc/tcp-brutal-config','/var/lib/tcp-brutal-config']:
    d=pathlib.Path(p)
    if d.exists():shutil.rmtree(d)
PY
systemctl daemon-reload
echo "服务：$SERVICE；配置：$CONFIG；程序：$PREFIX/tcp-brutal-manager"
