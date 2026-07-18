# Seedance Domestic 部署与接入

本文说明如何部署渠道类型 59（Seedance Domestic），以及如何通过 `hwdrama-proxy` 公开五个素材接口。示例上游为 `https://api.laomandi.com`。

## 公网 IP HTTPS 入口

需要在域名尚不可用时直接通过公网 IP 提供服务，可使用 [seedance-domestic-caddy.example](seedance-domestic-caddy.example) 中的 Caddy 配置。该配置让域名和公网 IP 复用完全相同的 API、素材代理和 `/apidocs/` 路由，同时为公网 IP 向 Let's Encrypt 申请受信任的短期证书。

部署时必须满足：

- Caddy 支持 ACME `profile shortlived`，本次生产验证使用 `v2.11.4`。
- TCP 80、443 均能从公网访问；80 用于 ACME HTTP-01 校验，不能只开放 443。
- Caddy 的 `/data` 使用持久卷，否则重建容器会丢失证书状态和 ACME 账户。
- 保留全局 `default_sni 124.174.0.221`。多数客户端连接字面 IP 时不会发送 SNI；没有该项时，即使证书已签发，`https://124.174.0.221` 仍可能在 TLS 握手阶段失败。
- IP 证书有效期约 6 天，必须由 Caddy 持续自动续期，不能使用人工复制后长期不维护的证书文件。

修改 Caddyfile 后先执行配置校验，再重载或重建 Caddy。客户端验证不得使用 `-k`：

```bash
docker exec new-api-seedance-caddy-1 caddy validate --config /etc/caddy/Caddyfile
curl --fail --show-error https://124.174.0.221/api/status
curl --fail --show-error https://124.174.0.221/apidocs/
```

不要用明文 `http://<IP>` 传输 API Key，也不要用 `tls internal` 作为客户公网入口；前者会泄露凭证，后者要求所有客户额外安装私有根证书。

## 1. 创建 new-api 渠道

在管理后台新建渠道并填写：

| 配置项 | 值 |
| --- | --- |
| 渠道类型 | `59 - Seedance Domestic` |
| Base URL | `https://api.laomandi.com` |
| 密钥 | 供应商分配的 `lmd-key` 值，只填写密钥本身 |
| 模型 | `doubao-seedance-2-0-260128` |
| 状态 | 启用 |

渠道创建后记下它的数据库 ID。动态素材路由中的 `channel_id` 必须填写这个数据库 ID，不能直接填写渠道类型 `59`。

该适配器会把 new-api 的 `Authorization` 鉴权转换为供应商要求的 `lmd-key` 请求头。请勿在客户端请求中直接携带供应商密钥。

## 2. 创建视频任务

客户端仍通过 new-api 的统一端点提交任务。下面展示特殊 `content` 格式，并引用一个已经处理为 `Active` 的素材：

```bash
curl -X POST "https://NEW_API_HOST/v1/video/generations" \
  -H "Authorization: Bearer NEW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-260128",
    "content": [
      {
        "type": "text",
        "text": "图片1中的人物在雨后的街道缓慢向前走，电影感运镜"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "asset://asset-2026-example"
        },
        "role": "reference_image"
      }
    ],
    "audio_status": 1,
    "resolution": "720p",
    "ratio": "16:9",
    "dur": 5
  }'
```

参数约束：

- `resolution` 已实现 `720p`、`1080p`、`4k` 转发。客户 OpenAPI 暂只公布已完成端到端验证的 `720p`、`1080p`；`4k` 必须在确认实际渠道已开通并完成端到端验证后再对客户开放。4K 输出为 10-bit H.265，调用方还需要确认播放环境支持该编码。
- `ratio` 支持 `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive`。
- `dur` 支持 4–15 的整数，或 `-1` 让模型在 4–15 秒内决定时长。
- `audio_status` 为 `1` 时生成同步音频，为 `0` 时生成无声视频。
- 素材必须先通过 `GetAsset` 确认为 `Active`，再以 `asset://<AssetId>` 形式放进 `image_url.url`。

保存创建响应中的 new-api 任务 ID。供应商的数值任务 ID 只在服务端保存，不向客户端暴露。

## 3. 查询任务

使用创建接口返回的 new-api 任务 ID 轮询：

```bash
curl "https://NEW_API_HOST/v1/video/generations/TASK_ID" \
  -H "Authorization: Bearer NEW_API_KEY"
```

new-api 在服务端调用供应商的 `POST /asset/SdToolApi/generate-info`，并把等待、生成中、成功或失败状态转换为统一任务响应。成功后请及时保存视频链接，因为供应商链接可能有有效期。

计费按任务创建时冻结的人民币单价、汇率和分组倍率执行。只有供应商账单确认成功的任务才最终结算；后台对账会用实际 `total_tokens` 修正预扣额度。

当前实现使用以下火山引擎 Seedance 2.0 人民币报价快照（单位：元/百万 token）：

| 分辨率 | 无输入视频 | 含输入视频 |
| --- | ---: | ---: |
| 720p | 46 | 28 |
| 1080p | 51 | 31 |
| 4K | 26 | 16 |

token 估算遵循 `(输入视频秒数 + 输出视频秒数) × 像素数 × 24 / 1024`。预扣时无法读取输入视频的真实时长，因此会按每个输入视频 15 秒、`dur=-1` 按输出 15 秒做保守估算；成功后再以私有账单的 `total_tokens` 对账。人民币成本通过任务创建时冻结的 `USDExchangeRate` 换算成 new-api 额度，分组倍率仍可用于销售加价或折扣。供应商账单中的 `price`、`discount`、`amount_paid` 仅保存用于成本审计，不会覆盖面向用户的官方报价快照。

4K 参数和价格依据火山方舟当前的 [创建视频生成任务 API](https://www.volcengine.com/docs/82379/1520757?lang=zh)、[模型列表](https://www.volcengine.com/docs/82379/1330310?lang=zh) 与 [模型价格](https://www.volcengine.com/docs/82379/1544106?lang=zh)。部分国内供应商的特殊封装文档仍只列出 720p/1080p；使用 4K 前需要确认实际渠道已同步开放该枚举，否则请求会被供应商拒绝。

## 4. 部署五个公开素材接口

动态路由示例位于 [seedance-domestic-hwdrama-routes.example.yml](seedance-domestic-hwdrama-routes.example.yml)。复制后至少替换：

- `api_key_ids`：允许调用素材接口的 new-api Token 数据库 ID。
- `channel_id`：步骤 1 创建的渠道数据库 ID。
- `upstream_api_key_env`：可保留示例变量名，也可换成自己的环境变量名。

五个客户端端点如下：

| 客户端端点 | 供应商端点 |
| --- | --- |
| `POST /api/v3/open/CreateVisualValidateSession` | `/asset/SdToolApi/CreateVisualValidateSession` |
| `POST /api/v3/open/GetVisualValidateResult` | `/asset/SdToolApi/GetVisualValidateResult` |
| `POST /api/v3/open/CreateAssetGroup` | `/asset/SdToolApi/CreateAssetGroup` |
| `POST /api/v3/open/CreateAsset` | `/asset/SdToolApi/CreateAsset` |
| `POST /api/v3/open/GetAsset` | `/asset/SdToolApi/GetAsset` |

生产环境端到端验证确认，这些接口使用 `{ "state": 1, "data": { ... }, "error": null }` 业务包装，而不是供应商文字示例中的根级字段：

- `CreateVisualValidateSession` 的认证结果位于 `data.Result`，响应元数据位于 `data.ResponseMetadata`。
- `CreateAssetGroup` 和 `CreateAsset` 的标识均位于 `data.Id`。
- `GetAsset` 的素材信息位于 `data`，其中包含 `Moderation.Strategy`；`CreateTime`、`UpdateTime` 为 ISO 8601 UTC，`Error` 字段可能缺失，临时签名 URL 有效期为 12 小时。
- `GetVisualValidateResult` 的无效令牌响应已确认是 HTTP 200、`state=0`、`data=null`、`error` 为字符串数组；成功路径需在具备真人认证条件后补做端到端验证。

因此 `CreateAsset` 必须配置 `affinity_response_field: data.Id`。该字段会记录素材 ID、new-api Token 和渠道的亲和关系；之后视频请求引用同一 `asset://` ID 时，分发器会固定选择创建该素材的渠道，防止素材被发往其他供应商账号。

该值是 gjson 字段路径，必须与线上响应完全一致。缺少配置字段时代理会拒绝成功响应，以免产生没有亲和关系的素材。若后续接口响应发生变化，应先完成端到端确认，再同步修改动态路由和客户 OpenAPI；不要仅依据已过时的文字示例调整路径。

`hwdrama-proxy` 和 new-api 主进程必须连接同一个主数据库。反向代理应只把上述五个 `/api/v3/open/*` 路径转发到 `hwdrama-proxy` 端口，视频生成和查询仍转发到 new-api 主进程。

## 5. 密钥与环境变量

不要把 `lmd-key` 写入 YAML 或镜像。可以通过进程环境变量提供：

```dotenv
HWD_PROXY_PORT=3001
HWD_PROXY_ROUTES_CONFIG=/opt/new-api/seedance-domestic-routes.yml
SEEDANCE_DOMESTIC_LMD_KEY=replace-with-supplier-secret
```

也可以把最后一项放入权限为 `0600` 的独立密钥文件，并设置：

```dotenv
HWD_PROXY_SECRETS_FILE=/opt/new-api/hwdrama-proxy-secrets.env
```

密钥文件内容：

```dotenv
SEEDANCE_DOMESTIC_LMD_KEY=replace-with-supplier-secret
```

YAML 中 `upstream_auth_header: lmd-key` 和空的 `upstream_auth_prefix` 表示原样写入 `lmd-key: <secret>`，不会添加 `Bearer`。客户端的 `Authorization` 请求头在转发前会被移除。

先验证配置，再启动或热重载代理：

```bash
/hwdrama-proxy config validate \
  --config /opt/new-api/seedance-domestic-routes.yml \
  --secrets /opt/new-api/hwdrama-proxy-secrets.env

curl -X POST http://127.0.0.1:3001/-/reload
curl http://127.0.0.1:3001/healthz
```

如设置了 `HWD_PROXY_ADMIN_TOKEN`，调用 `/-/reload` 时还需添加 `X-Hwd-Proxy-Admin-Token` 请求头。

## 6. 安全边界

`POST /asset/SdToolApi/ListSplitBillDetail` 仅供 new-api 后台对账任务使用，**不得加入动态路由、不得由 Nginx 公开、不得允许客户端调用**。账单响应含供应商成本与审计信息，公开后还可能绕过 new-api 的用户权限边界。

动态素材代理只允许 YAML 中声明的五个动作，并按 new-api Token ID 控制路由。不要使用 `all_api_keys: true`，除非已明确接受该租户范围。

## 7. 部署前检查

- 已完成数据库迁移，主进程与 `hwdrama-proxy` 使用同一数据库。
- 类型 59 渠道的 Base URL、`lmd-key` 和模型均已配置，并已用一条最小视频任务完成连通性测试（异步视频渠道不使用后台的同步渠道测试按钮）。
- YAML 中填写的是渠道数据库 ID 和 Token 数据库 ID，不是渠道类型或明文 API Key。
- `SEEDANCE_DOMESTIC_LMD_KEY` 已注入进程或 `0600` 密钥文件，日志和配置文件中没有明文密钥。
- 路由配置验证通过，五个公开路径能命中，`ListSplitBillDetail` 无公网路由。
- `CreateAsset` 响应字段路径与 `affinity_response_field` 一致，数据库能写入素材亲和记录。
- 已验证 `CreateAsset` → `GetAsset` 到 `Active` → `asset://` 创建视频的完整链路。
- 已验证任务成功后的实际 token 对账，以及失败任务不产生最终扣费。
- 生成视频 URL 和素材 URL 已按业务需要及时下载并持久化。
