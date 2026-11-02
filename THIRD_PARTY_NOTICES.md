# 第三方许可与素材

本项目基于 [LogicShao/Kairos](https://github.com/LogicShao/Kairos) 的 MIT 源码发展，保留其原始版权与许可，并注明维护者 [moonquake25104](https://github.com/moonquake25104) 的版权，见 [LICENSE](LICENSE)。

AndroidLiquidGlass / Backdrop 源码采用 Apache-2.0，位于 App/mobile/android/henubox-backdrop；保留原始文件头、来源清单和模块 assets 下的 NOTICE。通用许可在 third_party/licenses 中。React 等运行依赖的许可见下表，完整文本由应用内“开源致谢”展示，生成入口为 scripts/generate-open-source-notices.mjs。其他依赖仍适用各自包中的许可。

## 应用直接运行时依赖

| 依赖 | 许可 | 用途 |
|---|---|---|
| react / react-dom | MIT | 界面运行时 |
| @fluentui/react-components | MIT | 主题与基础组件 |
| radix-ui | MIT | 可访问性组件 |
| lucide-react | ISC | 图标 |
| @fontsource-variable/geist | OFL-1.1 | 字体 |
| class-variance-authority | Apache-2.0 | 组件样式变体 |
| clsx | MIT | className 合并 |
| tailwind-merge | MIT | Tailwind 类名合并 |
| tw-animate-css | MIT | 动画样式 |
| shadcn | MIT | Tailwind 基础样式 |
| vaso | MIT | 依赖（当前未直接引用） |
| @capacitor/core / @capacitor/android | MIT | Android 壳运行时 |
| @capacitor/app / local-notifications / network / preferences / status-bar | MIT | Android 壳插件 |

上表是 App/package.json 与 App/mobile/package.json 的直接运行时依赖。新增依赖时请同步扩展 scripts/generate-open-source-notices.mjs 和本表。

内置雨声、海浪、粉噪声、棕噪声的 CC0 来源与处理说明见 App/mobile/android/app/src/main/assets/focus-audio/NOTICE.txt。ZZUPocket、Tomato、Goodtime 为交互参考，未将其 GPL 应用代码或素材复制进专注功能。

**代码的 MIT 许可不授予第三方图片、字体、校徽、校歌、地图、网页或商标的额外权利。** 默认首页图片由项目使用者提供，公开副本以程序绘制的中性占位图替代，避免传播来源未确认的图像。应用图标及其原始参考图同样替换为本仓库自行绘制的盒子图案；学校公开资料通过官方链接按需访问，未附带个人课表、抓包或学校账号。

