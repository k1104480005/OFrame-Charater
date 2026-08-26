# 推送到 GitHub 指引（PUSH.md）

本地仓库已完成提交并配置好远程，只差执行推送。以下任选一种方式。

## 0. 当前本地状态

- 分支：`master`
- 最新提交：`ad3bbe2 feat: OFrame Character workbench — generation→acceptance→export pipeline`
- 远程：`origin → https://github.com/k1104480005/OFrame-Charater.git`
- 工作区：干净

## 方式一：在有 GitHub 网络的环境直接推（最简单）

在能正常打开 github.com 的网络（如家用宽带/公司网络/VPN 已连通）下执行：

```bash
git push -u origin master
```

首次推送会要求认证（二选一）：

- **Git Credential Manager 弹窗**：浏览器登录 GitHub 授权即可；
- **Personal Access Token**：GitHub → Settings → Developer settings → Personal access tokens
  生成一个 `repo` 权限的 token，用户名填你的 GitHub 用户名，密码填 token。

## 方式二：配置 HTTP 代理后推送

如果你有可用的 HTTP/SOCKS 代理（能访问 GitHub），执行：

```bash
git config --global http.proxy http://<代理主机>:<端口>
git push -u origin master
```

推送成功后如需取消代理：

```bash
git config --global --unset http.proxy
```

## 方式三：用令牌直接推（一次性）

```bash
git push -u origin https://<用户名>:<PAT>@github.com/k1104480005/OFrame-Charater.git
```

推完后把远程地址改回不带令牌的形式，避免令牌残留在配置里：

```bash
git remote set-url origin https://github.com/k1104480005/OFrame-Charater.git
```

## 推送后确认

```bash
git status                    # 应显示 up to date / nothing to commit
git ls-remote origin          # 能看到远端 master 分支即推送成功
```

## 附：发布 Beta 时可一并上传的产物

```
build/bin/OFrameCharacterWorkbench.exe                      便携版
build/bin/oframe-character-workbench-amd64-installer.exe    NSIS 安装包
```

在 GitHub 仓库 Releases 页新建 Release 并上传这两个文件即可分发。
