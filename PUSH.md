# 推送到 GitHub 指引（PUSH.md）

本仓库的远程与认证方式见下文；**当前工作区有大量未提交改动**，推送前必须先完成提交。
完整流程见文末「提交当前改动并推送」一节。

## 0. 当前本地状态（以 2026-08-31 会话为准）

- 分支：`master`
- 最新提交：`2fc878b align style presets with perfectpixel and harden identity workflow`
- 远程：`origin → https://github.com/KANGKUNTAO/OFrame-Character.git`（2026-08-31 仓库改名并更新 remote；旧地址 `k1104480005/OFrame-Charater` 仍会重定向）
- git 身份：已配置（康坤涛 / k1104480005@users.noreply.github.com）
- 仓库级代理：`http.proxy socks5h://127.0.0.1:7897`（本机 Clash 系代理当前监听 7897；旧的 7890 已失效）
- 工作区：`2fc878b` 已推送至远端 master（`git ls-remote` 已核对）
- `.gitignore` 已覆盖：`/build/bin/`、`*.exe`、`*.db`、`frontend/dist/*`（保留 stub index.html）、`node_modules`、`.workbuddy/`
- `issue-link.txt` 是有意保留的未跟踪文件，正常提交即可，不要删除

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
git push -u origin https://<用户名>:<PAT>@github.com/KANGKUNTAO/OFrame-Character.git
```

推完后把远程地址改回不带令牌的形式，避免令牌残留在配置里：

```bash
git remote set-url origin https://github.com/KANGKUNTAO/OFrame-Character.git
```

## 推送后确认

```bash
git status                    # 应显示 up to date / nothing to commit
git ls-remote origin          # 能看到远端 master 分支即推送成功
```

## 附：提交当前改动并推送（标准流程）

推送前必须先提交工作区中的改动。按以下顺序执行：

1. **提交前检查**（不绿不许提交）：

   ```powershell
   go test -count=1 ./...
   cd frontend; pnpm run typecheck; pnpm run build; cd ..
   ```

2. **审阅改动**：`git status --short` 与 `git diff` 过一遍；把暂存后的 diff 再看一遍，
   确认没有 API key / 令牌 / 本地路径等敏感信息（Provider 的 API key 存在仓库外的
   应用配置目录，正常不会出现在仓库里——若 diff 中出现任何密钥，停下排查来源）。

3. **提交**（本仓库提交信息风格：小写开头、简短祈使句，参考 `git log --oneline`）：

   ```powershell
   git add -A
   git commit -m "integrate perfectpixel workflow: identity source lock, model display, style alignment"
   ```

   （可按实际改动拆成多个提交；单提交也可接受。不要使用 --no-verify 绕过钩子。）

4. **推送**：按上文「方式一 / 二 / 三」任选其一执行 `git push -u origin master`。
   经验提示：本机网络直连 github.com 曾失败过——超时或拒绝连接时改用方式二配置代理，
   或方式三令牌直推；令牌直推后务必 `git remote set-url` 改回干净地址。

5. **推送后确认**：`git ls-remote origin` 能看到远端 master 指向刚才的提交即成功；
   可顺带在 GitHub Releases 上传 `build/bin/` 下两个产物（见下文）。

## 附：发布 Beta 时可一并上传的产物

```
build/bin/OFrameCharacterWorkbench.exe                      便携版
build/bin/oframe-character-workbench-amd64-installer.exe    NSIS 安装包
```

在 GitHub 仓库 Releases 页新建 Release 并上传这两个文件即可分发。
