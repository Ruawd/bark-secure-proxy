# Bark Secure Proxy

一个轻量级的 Bark 请求加密转发服务。它位于业务系统与原生 `bark-server` 之间，负责：

- 统一管理设备的加密参数（encodeKey / IV / 状态）
- 按 Bark App 要求自动生成 AES-CBC/PKCS7 密钥
- 将明文通知体加密后再转发给已有的 `bark-server`
- 查询、激活、停用设备，并提供简单健康检查

与 `E:\bark\bark-api` 相同的业务诉求，但使用 Go 实现、零外部依赖（BoltDB 文件存储），部署更简单。

## 快速开始

```powershell
cd E:\bark\bark-secure-proxy
copy config.example.yaml config.yaml   # 根据环境修改 bark.base_url、storage.path 等
go run ./cmd/bark-secure-proxy -config config.yaml
```

默认服务监听 `:8090`，会在 `storage.path` 指定的路径创建 BoltDB 数据库保存设备信息。

## 可视化前端 & 管理后台

- 前端入口：`http://<proxy-host>:8090/`
- 默认开启登录（账号密码在 `config.yaml` → `auth` 配置，可设为 bcrypt hash 或明文）；登录成功后可访问仪表盘、设备管理、推送中心、日志与接入向导
- 前端完全基于本项目的静态文件，可通过 `frontend.dir` 指向自定义构建产物

## Bark App 接入流程

1. **在 Bark App 中添加服务器**：把 App「私人服务器」地址指向本代理（例如 `https://proxy.example.com`）。App 会调用 `/ping`、`/register` 等接口，本代理会自动转发到真实 `bark-server` 并缓存 `deviceKey`。
2. **生成加密配置**：App 完成注册后，继续调用 `/device/gen` 生成/更新 encodeKey 与 IV，可传入已有值，也可留空让服务端随机生成。响应体中会返回完整的设备配置，直接复制到 Bark App -> 设置 -> 加密设置。

示例：

```http
POST /device/gen
Content-Type: application/json

{
  "deviceToken": "从 Bark App 设置里复制",
  "name": "我的 iPhone",
  "deviceKey": "App 首页显示的 deviceKey",
  "algorithm": "AES",
  "model": "CBC",
  "padding": "PKCS7Padding",
  "encodeKey": "",
  "iv": ""
}
```

3. **发送通知**：可以继续使用老的 GET 方式（`/notice/title/body`、`/notice?title=...&body=...`），也可以 `POST /notice`：

```http
POST /notice
Content-Type: application/json

{
  "title": "来自自建服务器",
  "body": "你好 Bark",
  "group": "demo",
  "deviceKeys": ["ExSJRzFV9yRYEsDh4fAXM4"] // 省略则群发所有 ACTIVE 设备
}
```

## 配置说明（config.yaml）

| 配置段         | 说明                                                                 |
| -------------- | -------------------------------------------------------------------- |
| `http`         | 监听地址以及读写超时                                                  |
| `bark`         | 已部署好的 `bark-server` 地址、API Token（如果启用了 server token）    |
| `storage`      | 设备信息落盘文件路径                                                 |
| `crypto`       | 默认算法/模式/填充与自动生成的 key、iv 长度（默认为 32/16 字符）      |
| `frontend`     | 静态页面目录，代理启动时会自动托管该目录下的文件                      |
| `auth`         | 管理后台登录开关、默认账号密码、JWT 密钥                              |

可直接修改 `config.example.yaml` 后作为 `config.yaml` 使用。

## 主要 API

### 设备 / 注册

| Endpoint | Method | 请求示例 | 返回 |
| --- | --- | --- | --- |
| `/ping` (透传 Bark) | GET | `curl http://proxy/ping` | Bark Server JSON |
| `/register` | GET | `curl "http://proxy/register?devicetoken=xxx&key=oldKey"` | `{"code":200,"data":{"device_key":...}}` |
| `/device/gen` | POST `application/json` | `{"deviceToken":"...", "deviceKey":"...", "name":"iPhone"}` | `{"code":"000000","data":{"encodeKey":"...","iv":"..."}}` |
| `/device/query` | GET | `curl "http://proxy/device/query?deviceToken=xxx"` | 设备详情 |
| `/device/queryAll` | GET | - | `DeviceConfDTO[]`（敏感字段打星） |
| `/device/active` `/device/stop` | GET | `curl "http://proxy/device/active?deviceToken=xxx"` | 成功/失败 |

### 推送

| Endpoint | Method | 说明 |
| --- | --- | --- |
| `/notice` | GET | 兼容旧式 `?title=...&body=...` |
| `/notice/:title/:body` | GET | Path 传参 |
| `/notice` | POST | `{"title":"","body":"必填","group":"可选","deviceKeys":["可选"]}`，若不传 `deviceKeys` 则群发 ACTIVE 设备 |

可选字段包括 `subtitle`、`url`、`icon`、`image`，返回值统一：`{"code":"000000","msg":"发送成功","data":{"sendNum":N,"successNum":M}}`。

### 日志 / 状态

| Endpoint | 说明 |
| --- | --- |
| `/api/notice/log/list?page=1&pageSize=20&group=&status=&beginTime=&endTime=` | 返回 `{"data":[NoticeLog], "total":...}`，需要 `Authorization: Bearer <token>` |
| `/api/notice/log/count/{date,status,group,device}` | 统计各维度数量 |
| `/status/endpoint` | 需 `API-TOKEN` 头（值为 `config.yaml` 中 `bark.token`），返回 `{"status":"在线","activeDeviceNum":1,"allDeviceNum":2}` |

### 管理后台内部接口

- `/auth/login` `POST {"username":"","password":""}` → `{"token":"..."}`
- `/admin/summary`、`/admin/devices` 等仅管理页调用，需要 Bearer Token。

所有 `/device/*`、`/notice`、`/api/notice/log/*` 等接口都会返回与 `E:\bark\bark-api` 相同的 `BasicResponse`（`code/msg/data`），现有脚本可以直接切换到该代理而无需改动。

## API 设计

### 1. 健康检查

```
GET /healthz
```

若配置了 Bark 客户端，会同时探测 `bark-server` /ping。

### 2. 设备管理

```
POST /devices
```

请求体：

```json
{
  "deviceToken": "从 Bark App 拿到的deviceToken",
  "deviceKey": "",          // 可选，留空则自动向 bark-server 注册获取
  "name": "Living Room",
  "encodeKey": "",          // 可选，留空自动生成 32 字符
  "iv": "",                 // 可选，留空自动生成 16 字符
  "status": "ACTIVE",       // ACTIVE / STOP
  "registerKey": ""         // 当需要沿用旧 deviceKey 时传入
}
```

响应返回完整设备信息（包含 encodeKey / iv，便于复制到 Bark App「加密设置」）。

```
GET /devices          # 列出所有设备
GET /devices/:token   # 查看单个设备
```

### 3. 推送接口

```
POST /notice
```

```json
{
  "title": "test title",
  "subtitle": "optional subtitle",
  "body": "内容必填",
  "group": "demo",
  "url": "https://...",
  "deviceKeys": ["xxxx", "yyyy"]   // 留空则对全部 ACTIVE 设备广播
}
```

代理会将明文序列化后按设备各自的 `encodeKey`/`iv` 加密，向 `bark-server` 推送，返回每台设备的成功/失败状态。

## 数据存储

- 使用 BoltDB（单一文件），路径由 `storage.path` 决定，默认 `./data/devices.db`
- 设备字段包括 `deviceToken / deviceKey / encodeKey / iv / status / timestamps`

## 构建与部署

```powershell
go build -o bin/bark-secure-proxy.exe ./cmd/bark-secure-proxy
.\bin\bark-secure-proxy.exe -config config.yaml
```

可以配合 Windows 服务 / NSSM / systemd 等守护运行。

## Docker

### 手动构建
```powershell
docker build -t bark-secure-proxy:latest .
docker run -d --name bark-secure-proxy -p 8090:8090 `
  -v ${PWD}/config.yaml:/app/config.yaml `
  -v ${PWD}/data:/app/data `
  bark-secure-proxy:latest
```

### docker compose
项目自带 `docker-compose.yml`：
```powershell
docker compose up -d
docker compose up -d --build   # 更新镜像
docker compose logs -f
```

## GitHub Actions 构建镜像

`.github/workflows/docker.yml` 会在 push 到 `main`/`master` 时自动执行。使用前需在仓库 Settings → Secrets → Actions 中添加：

| Secret | 说明 |
| ------ | ---- |
| `DOCKERHUB_USERNAME` | Docker Hub（或目标注册表）用户名 |
| `DOCKERHUB_TOKEN`    | Docker Hub Access Token / PAT |

推送后即可在注册表中获取 `bark-secure-proxy:latest` 镜像，服务器上 `docker pull <用户名>/bark-secure-proxy:latest` 再配合 `docker compose up -d` 即可部署。

## 后续可扩展方向

1. 增加操作审计与推送日志
2. 支持 Redis/MySQL 等其他存储后端
3. 提供简单前端页面方便手动录入设备
