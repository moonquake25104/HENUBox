# 开发与构建

## 工具与模块

Windows 10/11、PowerShell 5.1+、Node.js 22.12+、npm、Go 1.26+、JDK 21；Android SDK platform 35、Build Tools 36.0.0、NDK 27.1.12297006。Gradle Wrapper 8.11.1、AGP 8.7.2、Kotlin 2.2.20、Compose 1.8.3 随源码锁定。原生解析默认只构建 arm64-v8a，最低 Android 9/API 28；模拟器若采用 x86_64，需要自行扩展解析 AAR 的目标 ABI。

`App/server/go.mod` 本身声明 Go 1.25；`App/server/mobile/bind` 与根 `go.work` 使用 Go 1.26，因此完整移动构建仍按 1.26+ 准备。

JAVA_HOME、ANDROID_HOME 必须由开发者在当前终端设置；没有全局环境变量修改。scripts/project-env.ps1 将当前构建的临时文件、用户配置、缓存和调试签名重定向到源码目录的 .local。脚本默认使用官方 Gradle 仓库；可选 XIQUE_MAVEN_MIRROR=1 启用 Gradle 文件中已列出的阿里镜像。

可选 HENUBOX_CACHE_ROOT 允许显式指定自己的缓存目录；未设置时使用 .local/cache。它只复用下载/编译缓存，不共享两个 npm 模块的 node_modules。官方 Gradle 下载若受网络限制，可将校验和一致的官方压缩包放入自己的 Wrapper 缓存；不可关闭 distributionSha256Sum 校验。

## Android

执行根 README 的 build-android.ps1 -InstallDependencies。该入口依次运行各模块 npm ci、必要时编译 Go AAR、生成许可清单、TypeScript 检查、Vite 构建、Capacitor 同步、生成本机专属调试密钥并 assembleDebug。首次 Go 移动工具使用绑定模块锁定的 golang.org/x/mobile 版本。

源码不提交预编译 AAR、调试证书、APK、CA 或账号。调试密钥在 .local/signing；默认调试口令只用于本机新建密钥，不得用作正式发布签名。正式发布需维护者自行管理签名和版本升级，不能使用他人的调试密钥。

## 界面开发

在根目录先执行 . ./scripts/project-env.ps1，然后分别进入 App 和 App/mobile 执行 npm ci。网页预览在 App 运行 npm run dev；它默认走网页后端分支，与手机本机分支不同。移动资源构建入口 scripts/build-mobile-web.ps1 使用 vite.mobile.config.ts。

## 测试

纯解析测试入口 scripts/test-parser.ps1 不需要个人课表和数据库。Android 本地 JVM 测试在完成 cap sync 和解析 AAR 构建后，从 App/mobile/android 执行 ./gradlew.bat --no-daemon :app:testDebugUnitTest。构建 APK 使用 :app:assembleDebug，不使用会尝试单独打包所有库的根 assembleDebug。真机测试必须由设备拥有者授权，不在构建时自动连接或安装。

可选服务端 App/server 的 go test ./... 包含数据库集成测试与平台相关权限测试；缺测试库会跳过相应场景，Windows 文件权限与 Unix 模式可能不同。私人样本测试仅在明确提供环境变量时运行，样本不随源码发布。

## 可选网页后端

Android 本地模式不要求服务器。若开发历史网页/API 模块，App/.env.example 提供变量说明，App/compose.yaml 提供容器结构；自行设置数据库与登录密钥后再运行。未带入个人数据库和运行配置，后端启动会自动应用迁移，必须面向独立开发库。桌面学校授权 helper 在 App/connectors/henu-browser，为可选后端服务使用，不是 Android 登录前提。

