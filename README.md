# HENUBox

河小盒：面向河南大学校园生活的 Android 工具箱。课表首次保存到手机后，可以离线查看整学期；联网时可由用户登录教务查询课表更新与空闲教室。

这是学习交流项目，非学校官方应用，不含广告。学校网页与服务可能变化，信息以官方系统为准。当前以修复问题、完善体验和发布维护为主。公开仓库地址：**https://github.com/moonquake25104/HENUBox**。

## 仓库概览

| 模块 | 路径 | 用途 | 说明 |
|---|---|---|---|
| 界面 | App/src | 首页、课表、校园工具、设置和三种外观 | [界面说明](App/README.md) |
| Android | App/mobile/android | 本机存储、授权、通知、专注、嵌入浏览器与导航 | [Android 说明](App/mobile/README.md) |
| Go | App/server | 纯 HTML 解析、移动绑定及可选网页后端 | [Go 说明](App/server/README.md) |

课表支持 18 周、5/7 天显示、校区作息和个人编辑；校园工具包含空闲教室、图书馆与邮箱官方网页、校园指南、专注和待办。电费查询仍为预留入口。“我的 → 关于HENUBox”可手动检查 GitHub Releases 最新版本；只显示版本与更新说明并在浏览器打开，不自动下载或安装。

## 快速开始

需要 Windows PowerShell、Node.js 22.12+、Go 1.26+、JDK 21、Android SDK 35、Build Tools 36.0.0 和 NDK 27.1.12297006。先在当前终端设置 JAVA_HOME 和 ANDROID_HOME（可选 ANDROID_NDK_HOME），指向自己的安装目录。

1. 下载源码并在根目录打开 PowerShell。
2. 执行下列命令，安装两个模块各自的依赖并构建：

~~~powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-android.ps1 -InstallDependencies
~~~

3. 安装 App/mobile/android/app/build/outputs/apk/debug/app-debug.apk，再由本人完成学校登录并选择校区。重新签名构建不能覆盖其他签名的安装包，勿直接卸载仍有个人数据的旧应用。

首次构建需要下载公开依赖，具体工具版本、独立测试和可选网页开发见 [开发说明](docs/BUILDING.md)。

## 常用命令

~~~powershell
# 单独运行纯解析测试
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-parser.ps1
# 重建移动界面（先安装 App 和 App/mobile 依赖）
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-mobile-web.ps1
# 重建 Go 解析 AAR
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-parser.ps1
~~~

命令来源：scripts 中的 PowerShell 入口及各模块 package.json、go.mod；工具配置只在当前进程生效。

界面代码静态检查可在 App 目录运行 npm run lint（来源：App/package.json）。这与 TypeScript 构建检查是不同的检查入口。

## 目录结构

| 路径 | 内容 |
|---|---|
| App/ | 当前界面、Android、解析器与可选后端源码 |
| scripts/ | 可移植构建和测试入口 |
| docs/ | 构建、隐私、发布与贡献说明 |
| third_party/ | 第三方许可原文 |

依赖、临时文件与缓存留在 .local 或模块自己的 node_modules，构建资源留在 runtime/build；均被 Git 忽略，不包含私人密钥、账号、课表或历史运行目录。

## 命名说明

对外品牌统一为 HENUBox。Android `applicationId`（`com.xique.assistant`）、Go 模块路径（`kairos/server`）、npm 包名及部分历史组件名沿用早期内部标识；首次公开发布后不再变更 `applicationId`，以免影响已安装用户的覆盖升级。

## 隐私与使用说明

课表、待办、专注记录和自选媒体主要保存在设备；学校会话由 Android Keystore 保护，学校登录由用户在对应网页完成。图书馆、邮箱、全景等外部页面适用其各自政策。通知、提醒、勿扰等权限按需申请。详见 [隐私说明](docs/PRIVACY.md) 与应用内“我的 → 关于”。不承诺不存在漏洞，请勿在问题反馈中附带凭据或个人课表。

## 许可与致谢

代码沿用上游 [MIT 许可证](LICENSE)，保留 Kairos 原始版权，并注明 HENUBox 维护者 [moonquake25104](https://github.com/moonquake25104) 的版权。第三方源码、依赖与素材的许可分别列在 [第三方致谢](THIRD_PARTY_NOTICES.md)，并可在应用内离线查看。公开副本使用中性占位插画和盒子图标，避免携带来源未确认的私人素材。

## 维护与贡献

提交改动前阅读 [贡献说明](CONTRIBUTING.md)，安全问题见 [安全反馈](SECURITY.md)，首次发布和上传步骤见 [发布说明](docs/PUBLISHING.md)。下载与反馈以 GitHub 仓库页面为准。修改功能、权限、数据存储、依赖、构建脚本或许可时，应同步更新 README、开发说明和应用内关于内容。

