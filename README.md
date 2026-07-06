# VoHive

[![License: PolyForm Noncommercial 1.0.0](https://img.shields.io/badge/License-PolyForm--Noncommercial--1.0.0-blue.svg)](https://polyformproject.org/licenses/noncommercial/1.0.0)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](go.mod)
[![Vue 3](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js)](web/package.json)

> 面向高通 4G/LTE/5G 模组（Quectel EC20/EC25/EC21/EG25/EM20 等）的综合管理与代理服务平台。

VoHive 把模组热插拔管理、SOCKS5/HTTP 代理编排、短信收发、VoWiFi/IMS 通话、eSIM 全生命周期管理整合到一个服务里，并提供一套现代化的响应式 Web 管理后台。

## 核心特性

| 模块 | 说明 |
| --- | --- |
| 多模组并发管理 | USB 热插拔自动发现（ttyUSB 等）、多设备实时状态监控 |
| 轻量级代理引擎 | 内建 SOCKS5 / HTTP 代理内核，支持多实例并发；基于 `SO_BINDTODEVICE` 按设备网卡严格绑定出站流量 |
| 通信与短信中心 | 统一界面/API 处理 AT 短信收发、会话与联系人管理、USSD 交互，短信落库可查 |
| eSIM 管理 | 通过 AT 指令通道直接管理 eSIM 芯片，支持 Profile 下载、启用/停用、重命名、删除 |
| 全渠道通知 | 重要短信及系统告警可推送至 Telegram、Email、PushPlus、Bark、飞书（Lark/Feishu）、QQ 等 |
| ARM64 Docker 部署 | 从当前源码构建前端与 Go 后端，发布 `linux/arm64` 镜像到 GHCR |

## ARM64 iStoreOS Docker 安装

以下命令只创建本地容器配置，不会连接或修改路由器之外的任何外部设备。iStoreOS 部署默认使用 `network_mode: host`，这是为了让 VoHive 直接看到 `wwan0`、`usb0` 等蜂窝网卡；因此不再使用 Docker `ports` 端口映射。

```sh
mkdir -p /opt/vohive
cd /opt/vohive

curl -fsSL -o compose.yaml https://raw.githubusercontent.com/neocj/vohive-test/main/compose.yaml
curl -fsSL -o .env.example https://raw.githubusercontent.com/neocj/vohive-test/main/.env.example
cp .env.example .env
vi .env

# 在 .env 中设置强密码，不要继续使用 admin/admin：
# VOHIVE_ADMIN_PASSWORD=替换为你自己的强密码

# 在 .env 中设置实际 USB 模组设备节点：
# VOHIVE_SERIAL_DEVICE=/dev/ttyUSB2
# VOHIVE_CONTROL_DEVICE=/dev/cdc-wdm0

mkdir -p config data logs
docker compose pull
docker compose up -d
```

如果没有在 `.env` 中设置 `VOHIVE_ADMIN_PASSWORD`，Compose 会拒绝启动；如果程序最终读取到的 Web 密码仍是默认 `admin`，后端也会拒绝启动并打印修改提示。请不要把真实密码提交到仓库。

如果需要让 VoHive 访问指定串口或控制设备，只编辑 `.env` 中的变量，不要映射整个 `/dev`，不要挂载 Docker Socket，也不要启用 `privileged`：

```sh
VOHIVE_SERIAL_DEVICE=/dev/ttyUSB2
VOHIVE_CONTROL_DEVICE=/dev/cdc-wdm0
```

启动后查看状态：

```sh
docker compose ps
docker compose logs -f vohive
```

由于使用 host 网络，Web 服务会在 iStoreOS 主机网络上监听 `7575`。如需从局域网访问 Web 管理界面，只能通过 iStoreOS 防火墙允许 LAN 访问 `7575`；禁止允许 WAN 访问，也不要添加任何公网端口转发。

USB 模组拔插后，如果 `/dev/ttyUSB*` 或 `/dev/cdc-wdm*` 设备节点发生变化，请先更新 `.env` 中的 `VOHIVE_SERIAL_DEVICE` / `VOHIVE_CONTROL_DEVICE`，再重启容器：

```sh
docker compose restart vohive
```

## 构建说明

Docker 镜像完全从仓库当前源码构建：

- 前端源码位于 `web/`，使用 `npm ci` 和 `npm run build` 构建。
- Go 源码、`go.mod`、`go.sum` 位于仓库根目录。
- `github.com/iniwex5/vowifi-go` 使用 `third_party/vowifi-go` 本地替换，来源固定为公开仓库 `boa-z/vowifi-go` 的提交 `f6eff2c27014e7d17e3660e32ca727fb04ca91b6`，构建时不跟随 `main` 或 `latest`。
- Dockerfile 不引用 `go-4gproxy` 目录。
- 构建不使用 `GH_PAT`，不配置 `GOPRIVATE`，不访问私人仓库，不下载预编译 VoHive 二进制。
- 在线自更新和自毁卸载接口已禁用，不会从 `iniwex5/vohive-release` 或任何远程地址下载二进制并替换自身，也不会通过 Web 接口删除配置、数据、日志或可执行程序。
- GitHub Actions 手动触发后发布 `ghcr.io/neocj/vohive-test:arm64` 和 `ghcr.io/neocj/vohive-test:latest-arm64`，并启用 provenance 与 SBOM。

本地构建 ARM64 镜像：

```sh
docker buildx build --platform linux/arm64 -t ghcr.io/neocj/vohive-test:arm64 --load .
```

验证 Go 依赖公开可下载：

```sh
GOPRIVATE= GONOPROXY= GONOSUMDB= go mod download
```

## 持久化目录

Compose 默认使用以下目录：

| 主机目录 | 容器目录 |
| --- | --- |
| `./config` | `/app/config` |
| `./data` | `/app/data` |
| `./logs` | `/app/logs` |

## 免责声明

- **用途定位**：本项目主要面向个人学习、技术研究与功能测试场景，不建议直接用于生产环境或关键业务系统；由此产生的部署及使用风险由使用者自行承担。
- **非官方项目**：VoHive 为第三方独立开发的开源软件，与 Quectel（高通模组厂商）、高通公司及其他任何模组/芯片厂商均无官方关联、授权或合作关系，亦不对模组硬件本身的功能、质量或安全性负责。
- **合规使用**：使用本项目搭建的服务时，请自行确保符合所在地区的法律法规及电信运营商的服务条款，不得用于任何违法违规用途。因违规使用造成的一切法律责任由使用者自行承担，与本项目作者及贡献者无关。
- **无担保**：本软件按“现状”提供，不附带任何明示或暗示的担保，包括但不限于适销性、特定用途适用性及不侵权担保。因使用或无法使用本软件造成的任何直接或间接损失，作者及贡献者不承担任何责任。

## License

本项目基于 [PolyForm Noncommercial License 1.0.0](LICENSE) 开源，仅限非商业用途。如需商业授权，请联系作者另行协商。
