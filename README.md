# MyGoolApk

一个使用 Go 与 [NSGo](https://github.com/nsgo-dev/nsgo) 编写的第三方酷安（Coolapk）社区客户端，采用原生组件渲染与 Material Design 3 视觉风格，支持 Android、iOS 和 macOS。

> **注意**：本项目处于持续开发阶段，仅专注于社区内容浏览与互动，不包含应用市场（应用/游戏下载）功能。

## 主要特性

- **原生跨平台**：基于 Go + NSGo 原生 UI 渲染，支持 Android (`arm64`)、iOS (`arm64`) 与 macOS (`universal`)。
- **自适应布局**：手机上是底部导航栏；宽窗口改用侧边导航栏，平板和 Mac 上列表与详情左右并排；macOS 支持菜单栏快捷键、鼠标悬停与右键菜单。
- **首页与频道**：支持话题、关注、头条、热榜、数码、发现等服务端动态卡片与横滑切换，内置二手交易浏览。
- **帖子与互动**：支持图文、文章、酷图、问答、投票、点评等帖子类型；支持评论、楼中楼、点赞、收藏夹、发帖与富文本/表情渲染。
- **消息与账号**：支持网页登录、个人主页、通知中心（评论/回复/点赞/关注/私信/服务通知）、搜索与历史记录、深色模式。
- **图片查看器**：支持缩放、滑动切换、下拉/上推关闭、原图按需加载与保存。

## 构建产物说明

GitHub Actions 会在每次 CI 与标签发布（`v*`）时自动构建以下安装包：

| 平台 | 产物 | 说明 |
| --- | --- | --- |
| Android | `mygoolapk-<version>-android-arm64.apk` | `arm64-v8a` 安装包（若未配置签名密钥，默认使用调试签名） |
| iOS | `mygoolapk-<version>-ios.ipa` | 未签名 IPA，需使用 TrollStore、AltStore、SideStore、Sideloadly 或自签工具安装 |
| macOS | `mygoolapk-<version>-macos-universal.dmg` | Apple Silicon + Intel 通用镜像（Ad-hoc 签名，首次打开可能需要在系统设置中允许运行） |

## 本地构建与测试

### 环境要求

- Go（版本见 [`go.mod`](go.mod)）
- iOS / macOS 构建：macOS 与 Xcode

### 准备依赖目录

当前 `go.mod` 通过本地 `replace` 指向 [`CoolapkApi-Go`](https://github.com/XiaoMengXinX/CoolapkApi-Go) 与带补丁的 [`nsgo`](https://github.com/nsgo-dev/nsgo)：

```sh
# 假设在工作区根目录执行
git clone --depth 1 --branch iOS https://github.com/XiaoMengXinX/CoolapkApi-Go.git CoolapkApi-Go

mkdir -p coolapk-mygo && cd coolapk-mygo
git clone https://github.com/XiaoMengXinX/MyGoolApk.git app-nsgo

# 拉取对应版本的 NSGo 并打上补丁（详见 app-nsgo/patches/nsgo/README.md）
git clone --depth 1 --branch v0.3.4 https://github.com/nsgo-dev/nsgo.git nsgo
git -C nsgo am "$PWD"/app-nsgo/patches/nsgo/*.patch
```

### 运行测试与构建

```sh
cd app-nsgo

# 运行无头测试
go tool nsgo test ./...

# 构建 Android / iOS / macOS（产物输出到 dist/releases/）
go tool nsgo build -target android_arm64 .
go tool nsgo build -target ios .
go tool nsgo build -target macos_universal .
```

## 免责声明与许可协议

- 本项目为非官方第三方客户端，与酷安（Coolapk）官方无任何关联。
- MyGoolApk 自有源码采用 **Apache License 2.0 (Apache-2.0)**，详见 [`LICENSE`](LICENSE)。
- `CoolapkApi-Go` 的使用与分发授权由其权利人单独授予。
- 仓库中的酷安表情图片资源版权归原作者 / 酷安官方所有，仅用于社区内容展示。
