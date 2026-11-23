# HENUBox 官网工作区

这是 `https://henubox.ccwu.cc` 的静态介绍页源码，先在工作区开发和预览，确认后再同步到公开仓库 `D:\HENUBox` 的 `site/` 目录。

- 页面：`index.html`
- 样式：`styles.css`
- 交互：`app.js`
- 图片：`assets/`
- 自定义域名：`CNAME`
- 本地预览：在 `site` 目录运行静态 HTTP 服务，然后打开 `http://127.0.0.1:4173/`

部署方式：GitHub Pages → Source 选择 **Deploy from a branch**，分支 `main`，目录 `/site`。`henubox.ccwu.cc` 在 DNSHE 添加 CNAME 指向 `moonquake25104.github.io`，再在 Pages 设置里填写自定义域名并开启 HTTPS。
