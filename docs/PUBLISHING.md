# 发布到 GitHub

本文只说明首次公开发布的操作顺序；仓库里的源码、构建方式和隐私说明不因发布而改变。

## 0. 发布前检查

在仓库根目录执行：

```powershell
git status
git diff --cached --check
```

确认：

- 当前没有 `.env`、`*.jks`、`*.keystore`、`*.apk`、`*.aab`、`*.db` 等文件被暂存；
- `App/.env.example` 里只有占位值，没有真实密码或 token；
- 已经决定 `applicationId` 是否保持 `com.xique.assistant`（首次发布后再改会影响覆盖升级）；
- 至少跑过与改动相关的测试或构建（见 [开发与构建](BUILDING.md)）。

GitHub 不接受空目录，`App/deploy/certs/` 通过 `.gitkeep` 保留；证书本身不要提交。

## 1. 建立本地首次提交

如果还没有配置 Git 身份，请在**当前仓库**设置（不要用 `--global`，避免影响其他项目）：

```powershell
git config user.name "你的名字"
git config user.email "你的邮箱"
```

提交邮箱会写进公开的 Git 历史。若不想公开私人邮箱，请先在 GitHub 的 **Settings → Emails** 勾选 “Keep my email addresses private”，再用页面给出的 `...@users.noreply.github.com` 地址作为 `user.email`。

首次提交：

```powershell
git add -A
git commit -m "Initial commit: HENUBox"
git branch -M main
```

如果 `main` 还没有提交（首次发布时通常如此），执行后会得到一个干净的首次提交。提交前可以用 `git diff --cached --stat` 再看一眼文件清单。

## 2. 在 GitHub 建立空仓库

在浏览器打开 <https://github.com/new>：

1. **Repository name** 填 `HENUBox`；
2. 选择 **Public**；
3. **不要**勾选 “Add a README file”“Add .gitignore”“Choose a license”——这些文件仓库里已经有了，勾选会造成首次推送冲突；
4. 点击 **Create repository**。

创建后 GitHub 会显示仓库地址，例如：

```text
https://github.com/moonquake25104/HENUBox.git
```

## 3. 关联远程仓库并上传

使用 HTTPS：

```powershell
git remote add origin https://github.com/moonquake25104/HENUBox.git
git push -u origin main
```

如果使用 SSH（需要先在 GitHub 配置 SSH key）：

```powershell
git remote add origin git@github.com:moonquake25104/HENUBox.git
git push -u origin main
```

推送时如果要求登录，HTTPS 方式应使用 GitHub 的 **Personal Access Token**（Fine-grained token，至少给目标仓库 `Contents: Read and write`），不要使用账号密码。安装了 GitHub CLI 的话，也可以先执行 `gh auth login`，再用 `gh repo create` 建仓库并推送。

## 4. 发布后的仓库设置

- 在仓库首页右侧 **About** 填写一句简介，例如“河南大学校园生活 Android 工具箱”，并添加 `android`、`go`、`react`、`campus` 等 topics；
- **Settings → Security → Advisories** 开启私密漏洞报告，并把入口补到 `SECURITY.md`；
- 按需要开启 Issues、Discussions、分支保护（至少保护 `main`，要求 PR 或禁止 force push）；
- 在 **Releases** 创建 `v0.1.0`，附上自行签名构建的 APK；不要把签名密钥或 `.env` 上传到 Release；
- 仓库地址已按 `moonquake25104/HENUBox` 填写；如果你最终使用别的仓库名，请同步更新 `README.md`、`SECURITY.md`、`App/src/components/mobile/AboutLegal.tsx` 和本文中的链接。

## 5. 以后的更新

```powershell
git status
git add -A
git commit -m "描述这次改动"
git push
```

不要对已经公开的 `main` 使用 `git push --force`。如果推送被拒绝，先 `git pull --rebase`，解决冲突后再推送。

## 6. 常见问题

- **`remote origin already exists`**：先 `git remote -v` 查看，必要时 `git remote set-url origin <新地址>`。
- **推送提示文件过大**：检查是否误加了 `node_modules`、`build/`、APK 或视频素材；这些应在 `.gitignore` 中。
- **不想公开某段历史**：当前仓库还没有提交，这是最干净的时点；一旦推送后再改写历史会很麻烦。
- **想换仓库名**：GitHub 会自动重定向旧地址，但仍建议同步更新 README、`SECURITY.md` 和应用内链接。
