# Deploy P2P-IRC in one step

This is version **0.5.1**. One static binary. No Docker, no database, no
central server. Relays are still not part of the program.

本文说明如何一次装好、以后只敲 `p2pirc`。不要 Docker，不要中心服务器。

## 中文

### 一条命令

有 Go 1.22+ 时：

```bash
go install github.com/Andyccr/RainIRC/cmd/p2pirc@latest
p2pirc --lan --nickname Alice
```

没有 Go、或想装到 `~/.local/bin`（脚本会先下 Release 并校验 SHA-256）：

```bash
curl -fsSL https://raw.githubusercontent.com/Andyccr/RainIRC/main/scripts/install.sh | sh
p2pirc --lan --nickname Alice
```

从源码：

```bash
git clone https://github.com/Andyccr/RainIRC.git
cd RainIRC
make install          # → ~/.local/bin/p2pirc
# PREFIX=/usr/local make install
```

同一局域网里第二台机器同样 `--lan`。发现包验签通过后会自动连。组播被墙时再 `/connect host:port`。

### 只配一次

把常用开关写进 `~/.p2pirc/config`（命令行优先）：

```
# ~/.p2pirc/config
nickname=Alice
lan=true
```

之后直接：

```bash
p2pirc
```

身份仍在 `identity.json`，已知节点在 `peers.json`。不要把 `identity.json` 拷到不信任的机器。

可选的 systemd 用户服务：[contrib/p2pirc.service](../contrib/p2pirc.service)

```bash
mkdir -p ~/.config/systemd/user
cp contrib/p2pirc.service ~/.config/systemd/user/
systemctl --user enable --now p2pirc
```

服务默认执行 `p2pirc`（读 `~/.p2pirc/config`）。改 `ExecStart` 或写 config，不要改程序去连中心服务器。

### 发布包

`make dist` 交叉编译 linux/darwin/windows 的 amd64 与 arm64，并写 `dist/SHA256SUMS`。打 `v*` 标签后 GitHub Actions 上传 Release。

`install.sh` 在没有 Go 时下载对应 Release 文件。

## English

### One command

With Go 1.22+:

```bash
go install github.com/Andyccr/RainIRC/cmd/p2pirc@latest
p2pirc --lan --nickname Alice
```

Without Go (installs to `~/.local/bin`; verifies the Release SHA-256):

```bash
curl -fsSL https://raw.githubusercontent.com/Andyccr/RainIRC/main/scripts/install.sh | sh
p2pirc --lan --nickname Alice
```

From a clone:

```bash
make install                 # → ~/.local/bin/p2pirc
# PREFIX=/usr/local make install
```

`--lan` is `--auto-connect` plus `--reconnect`. A second machine on the same
LAN starts the same way. If multicast is blocked, `/connect host:port`.

### Configure once

Write `~/.p2pirc/config` (CLI flags override the file):

```
nickname=Alice
lan=true
```

Then `p2pirc` is enough. Keys stay in `identity.json`. Do not copy that file
to a machine you do not trust.

Optional systemd user unit: [contrib/p2pirc.service](../contrib/p2pirc.service)

### Release artifacts

`make dist` cross-compiles linux/darwin/windows amd64 and arm64 and writes
`dist/SHA256SUMS`. Pushing a `v*` tag publishes a GitHub Release. `install.sh`
fetches that binary when Go is not installed.
