# MyGoolApk

MyGoolApk是一款独立开发的第三方酷安客户端，支持Android、iOS和macOS。界面采用Material Design 3，使用自有API库[CoolapkApi-Go](https://github.com/XiaoMengXinX/CoolapkApi-Go)连接酷安。页面结构、交互和API均以官方酷安iOS 16.7.4为基准。

[下载最新版](https://github.com/XiaoMengXinX/MyGoolApk/releases/latest)

## 下载与安装

Android和macOS支持在应用内检查更新。

| 平台 | 产物 | 说明 |
| --- | --- | --- |
| Android | `mygoolapk-<version>-android-arm64.apk` | 适用于`arm64-v8a`。本地构建未指定签名密钥时使用调试签名。 |
| iOS | `mygoolapk-<version>-ios.ipa` | 未签名IPA，请使用设备支持的侧载工具安装，例如TrollStore、AltStore、SideStore或Sideloadly。 |
| macOS | `mygoolapk-<version>-macos-universal.dmg` | 支持Apple Silicon和Intel。使用ad-hoc签名，首次打开可能需要在系统设置中允许运行。 |

## 本地构建与测试

### 环境要求

- Go版本见[go.mod](go.mod)。
- 构建iOS和macOS版需要macOS与Xcode。
- 构建Android版需要JDK 17及以上、Android SDK Platform 36和Build Tools 36及以上。

### 准备依赖目录

`go.mod`通过本地`replace`引用CoolapkApi-Go的`iOS`分支和打过补丁的NSGo `v0.3.4`。克隆前请确认有权读取API库。[CoolapkApi-Go](https://github.com/XiaoMengXinX/CoolapkApi-Go)和[NSGo补丁说明](patches/nsgo/README.md)

```sh
# 从存放项目的父目录开始
git clone --depth 1 --branch iOS https://github.com/XiaoMengXinX/CoolapkApi-Go.git CoolapkApi-Go

mkdir -p coolapk-mygo && cd coolapk-mygo
git clone https://github.com/XiaoMengXinX/MyGoolApk.git app-nsgo

# 应用NSGo补丁
git clone --depth 1 --branch v0.3.4 https://github.com/nsgo-dev/nsgo.git nsgo
git -C nsgo am "$PWD"/app-nsgo/patches/nsgo/*.patch
```

构建Android版前需要重编NSGo的Android运行时。先将`ANDROID_HOME`设为本机Android SDK目录，然后从`coolapk-mygo/`执行：

```sh
sh nsgo/internal/platform/android/runtime/build.sh
```

### 运行测试与构建

```sh
cd app-nsgo

# 运行无头测试
go tool nsgo test ./...

# 按需选择目标平台，安装包输出到dist/releases/
go tool nsgo build -target android_arm64 .
go tool nsgo build -target ios .
go tool nsgo build -target macos_universal .
```

## 免责声明与许可协议

- 本项目与酷安官方无关联。
- MyGoolApk自有源码采用Apache License 2.0。[LICENSE](LICENSE)
- CoolapkApi-Go的使用与分发授权由其权利人单独授予。
- 仓库中的酷安表情图片版权归原作者或酷安官方所有，仅用于社区内容展示。
