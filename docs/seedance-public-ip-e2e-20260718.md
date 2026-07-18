# Seedance 2.0 公网 IP 端到端验证报告

## 1. 验证结论

- 验证入口：`https://124.174.0.221`
- 验证日期：2026-07-18（Asia/Shanghai）
- 部署实现：`453829ad`
- 鉴权方式：`Authorization: Bearer <Nexus Reach API Key>`
- 公开接口数量：9 个
- 付费视频生成次数：1 次
- 付费验证规格：720p、4 秒、无输入视频、无音频、使用素材库图片作为参考图
- 视频任务：`task_zzxWmWISHZRwfoTt7Mb4Mp66m9aRVsuj`
- 结果：视频创建、两种状态查询、完整下载、Range 下载、素材组创建、图片素材创建和素材查询均通过。
- 限制：`GetVisualValidateResult` 的成功分支要求用户在 120 秒内完成人工活体认证，本次自动化仅验证了会话创建成功和无效 BytedToken 的业务失败分支。

公网 IP 使用受信任的 IP SAN HTTPS 证书，客户端不需要 `-k`。域名 `gateway.nexus-reach.com` 的应用路由和证书配置已完成，但当前仍可能受备案/网络放行状态影响；在该问题解除前，以公网 IP HTTPS 入口为准。

部署后的零费用复验结果：`GetAsset` 返回 HTTP 200、`state=1`、`Status=Active`；100×100 图片的 `CreateAsset` 负例返回 HTTP 400 和 `InvalidParameter.WidthTooSmall`；两条素材响应均没有供应商 `Set-Cookie`；已有视频的 1-byte Range 请求返回 HTTP 206、`Content-Range: bytes 0-0/1684544` 和 `Cache-Control: private, no-store`。

## 2. 接口覆盖矩阵

| 顺序 | 公开 API | 验证内容 | 结果 |
| ---: | --- | --- | --- |
| 1 | `POST /api/v3/open/CreateVisualValidateSession` | 创建真人认证会话 | 通过 |
| 2 | `POST /api/v3/open/GetVisualValidateResult` | 无效 BytedToken 的 HTTP 200 业务错误 | 通过；成功分支需人工 |
| 3 | `POST /api/v3/open/CreateAssetGroup` | 创建素材组 | 通过 |
| 4 | `POST /api/v3/open/CreateAsset` | 从公网图片创建素材 | 通过 |
| 5 | `POST /api/v3/open/GetAsset` | 查询素材直至 `Active` | 通过 |
| 6 | `POST /v1/video/generations` | 使用素材库参考图创建最低价视频 | 通过 |
| 7 | `GET /v1/video/generations/{task_id}` | 国内格式状态轮询 | 通过 |
| 8 | `GET /v1/videos/{task_id}` | OpenAI 格式查询别名 | 通过 |
| 9 | `GET /v1/videos/{task_id}/content` | 完整下载和 Range 下载 | 通过 |

附加安全验证：公网 `POST /api/v3/open/ListSplitBillDetail` 返回 HTTP 404。该接口仅供网关内部结算使用，不是对外 API，也未写入 OpenAPI 文档。

## 3. 逐接口请求、响应与上游映射

以下所有公开 URL 均以 `https://124.174.0.221` 为基础地址。示例已删除 API Key、BytedToken、H5Link、上游 RequestId 和临时签名查询参数。

### 3.1 创建真人认证会话

- 公开 URL：`POST https://124.174.0.221/api/v3/open/CreateVisualValidateSession`
- 上游 URL：`POST https://api.laomandi.com/asset/SdToolApi/CreateVisualValidateSession`
- 转发：请求 JSON 原样转发；客户 `Authorization` 不发送给上游，代理改用服务端保存的 `lmd-key`。

请求：

```json
{
  "CallbackURL": "https://124.174.0.221/apidocs/apidocs-example-callback.html"
}
```

实际公开响应（HTTP 200）：

```json
{
  "state": 1,
  "data": {
    "ResponseMetadata": {
      "RequestId": "<redacted>",
      "Action": "CreateVisualValidateSession",
      "Version": "2024-01-01",
      "Service": "ark",
      "Region": "cn-beijing"
    },
    "Result": {
      "BytedToken": "<redacted>",
      "H5Link": "<redacted>",
      "CallbackURL": "https://124.174.0.221/apidocs/apidocs-example-callback.html"
    }
  },
  "error": null
}
```

### 3.2 获取真人认证结果

- 公开 URL：`POST https://124.174.0.221/api/v3/open/GetVisualValidateResult`
- 上游 URL：`POST https://api.laomandi.com/asset/SdToolApi/GetVisualValidateResult`
- 转发：请求和供应商业务响应原样转发。

本次自动化负例请求：

```json
{
  "BytedToken": "invalid-e2e-20260718T145325Z"
}
```

实际公开响应（HTTP 200）：

```json
{
  "state": 0,
  "data": null,
  "error": ["BytedToken信息有误"]
}
```

调用方不能只判断 HTTP 状态；业务成功条件是 `state === 1`、`data` 非空且 `error === null`。真人认证成功分支需在创建会话后的 120 秒内打开 H5Link 并由用户完成人工活体认证。

### 3.3 创建素材组

- 公开 URL：`POST https://124.174.0.221/api/v3/open/CreateAssetGroup`
- 上游 URL：`POST https://api.laomandi.com/asset/SdToolApi/CreateAssetGroup`
- 转发：请求和响应原样转发。

请求：

```json
{
  "Name": "nexus-ip-e2e-20260718T145325Z",
  "Description": "Public IP Seedance 2.0 E2E validation"
}
```

实际公开响应（HTTP 200）：

```json
{
  "state": 1,
  "data": {
    "Id": "group-20260718225334-bwxch"
  },
  "error": null
}
```

### 3.4 创建图片素材

- 公开 URL：`POST https://124.174.0.221/api/v3/open/CreateAsset`
- 上游 URL：`POST https://api.laomandi.com/asset/SdToolApi/CreateAsset`
- 转发：请求转发给与素材组绑定的同一供应商渠道；成功响应增加 `X-New-Api-Asset-Namespace: seedance-domestic`。

请求：

```json
{
  "GroupId": "group-20260718225334-bwxch",
  "URL": "https://images.clipsafari.com/6rhxknsi0s4gqoqot0z9u2593bf6?filename=cartoon-woman.png",
  "Name": "nexus-ip-e2e-virtual-avatar-20260718T145325Z",
  "AssetType": "Image"
}
```

实际公开响应（HTTP 200）：

```json
{
  "state": 1,
  "data": {
    "Id": "asset-20260718225340-wx6cz"
  },
  "error": null
}
```

已知的供应商素材业务错误由网关转换为 HTTP 400，并保留具体 `error.code` 和 `error.message`，避免改写成丢失原因的通用 502。

### 3.5 查询素材

- 公开 URL：`POST https://124.174.0.221/api/v3/open/GetAsset`
- 上游 URL：`POST https://api.laomandi.com/asset/SdToolApi/GetAsset`
- 转发：请求和业务响应原样转发；供应商 `Set-Cookie` 不向客户暴露。

请求：

```json
{
  "Id": "asset-20260718225340-wx6cz"
}
```

实际公开响应（HTTP 200，临时 URL 已移除签名参数）：

```json
{
  "state": 1,
  "data": {
    "Id": "asset-20260718225340-wx6cz",
    "Name": "nexus-ip-e2e-virtual-avatar-20260718T145325Z",
    "URL": "https://ark-media-asset.tos-cn-beijing.volces.com/<redacted>",
    "AssetType": "Image",
    "GroupId": "group-20260718225334-bwxch",
    "Status": "Active",
    "Moderation": {
      "Strategy": "Default"
    },
    "CreateTime": "2026-07-18T14:53:40Z",
    "UpdateTime": "2026-07-18T14:53:42Z",
    "ProjectName": "fzjktest6"
  },
  "error": null
}
```

只有 `Status=Active` 的素材可以用 `asset://<AssetId>` 参与视频生成。

### 3.6 创建视频任务

- 公开 URL：`POST https://124.174.0.221/v1/video/generations`
- 上游 URL：`POST https://api.laomandi.com/asset/SdToolApi/generate`
- 公开格式：国内 Seedance JSON 协议。
- 不支持：OpenAI/Sora 的 `POST /v1/videos` multipart 创建协议。

公开请求：

```json
{
  "model": "doubao-seedance-2-0-260128",
  "content": [
    {
      "type": "image_url",
      "image_url": {
        "url": "asset://asset-20260718225340-wx6cz"
      },
      "role": "reference_image"
    },
    {
      "type": "text",
      "text": "图片1中的虚拟人物自然眨眼并轻微呼吸，镜头固定，无文字。"
    }
  ],
  "audio_status": 0,
  "resolution": "720p",
  "ratio": "16:9",
  "dur": 4
}
```

上游请求会移除仅供网关选路的 `model` 字段，其余规范化内容保持一致：

```json
{
  "content": [
    {
      "type": "image_url",
      "image_url": {
        "url": "asset://asset-20260718225340-wx6cz"
      },
      "role": "reference_image"
    },
    {
      "type": "text",
      "text": "图片1中的虚拟人物自然眨眼并轻微呼吸，镜头固定，无文字。"
    }
  ],
  "audio_status": 0,
  "resolution": "720p",
  "ratio": "16:9",
  "dur": 4
}
```

公开响应返回网关生成的安全任务 ID。上游实际任务 ID 为 `2347`。由于网关创建时立即替换上游 ID，且后续轮询更新了保存的原始响应，本次没有为捕获上游原始创建响应而再次生成付费视频；根据实际保存的上游任务 ID，其创建成功响应契约为：

```json
{
  "state": 1,
  "data": {
    "id": 2347
  },
  "error": null
}
```

### 3.7 查询视频任务（国内格式）

- 公开 URL：`GET https://124.174.0.221/v1/video/generations/task_zzxWmWISHZRwfoTt7Mb4Mp66m9aRVsuj`
- 数据来源：网关任务数据库。
- 后台上游查询：`POST https://api.laomandi.com/asset/SdToolApi/generate-info`
- 后台上游请求：`{"id":2347}`

实际上游完成响应：

```json
{
  "state": 1,
  "data": {
    "id": 2347,
    "status": 2,
    "video_url": "https://s3.laomandi.com/file/<redacted>.mp4",
    "message": null,
    "created_at": "2026-07-18 22:53:46"
  },
  "error": null
}
```

实际公开完成响应：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_zzxWmWISHZRwfoTt7Mb4Mp66m9aRVsuj",
    "status": "SUCCESS",
    "fail_reason": "",
    "result_url": "https://s3.laomandi.com/file/<redacted>.mp4",
    "submit_time": 1784386426,
    "start_time": 1784386440,
    "finish_time": 1784386590,
    "progress": "100%"
  }
}
```

公开响应不会暴露数据库自增 ID、用户 ID、渠道 ID、额度字段或供应商私有属性。

### 3.8 查询视频任务（OpenAI 格式别名）

- 公开 URL：`GET https://124.174.0.221/v1/videos/task_zzxWmWISHZRwfoTt7Mb4Mp66m9aRVsuj`
- 数据来源：与国内格式查询相同的网关任务数据库。
- 上游调用：此 GET 本身不直接调用供应商；后台轮询负责更新任务。

实际公开响应：

```json
{
  "id": "task_zzxWmWISHZRwfoTt7Mb4Mp66m9aRVsuj",
  "task_id": "task_zzxWmWISHZRwfoTt7Mb4Mp66m9aRVsuj",
  "object": "video",
  "model": "doubao-seedance-2-0-260128",
  "status": "completed",
  "progress": 100,
  "created_at": 1784386426,
  "completed_at": 1784386590,
  "metadata": {
    "url": "https://s3.laomandi.com/file/<redacted>.mp4"
  }
}
```

该接口只是查询响应格式别名，不表示系统支持 OpenAI/Sora multipart 创建协议。

### 3.9 下载视频内容

- 公开 URL：`GET https://124.174.0.221/v1/videos/task_zzxWmWISHZRwfoTt7Mb4Mp66m9aRVsuj/content`
- 实现来源：Nexus Reach 本地代理接口，上游供应商没有同名 API。
- 上游访问：网关校验任务归属和成功状态后，流式读取任务保存的结果 URL。

完整下载结果：

- HTTP 200
- `Content-Type: text/plain`（供应商资源的响应头标注；文件内容实际为 MP4）
- `Cache-Control: private, no-store`
- 文件大小：1,684,544 bytes
- SHA-256：`7e9d720c0449ab495bfd9fd90d0bea1916db0ca0a9404dd9a30504e6f1a2d9f7`
- 文件头：`000000206674797069736f6d00000200`，包含 MP4 `ftyp/isom` 标识
- 视频：H.264，1254×720，24 fps，4.041667 秒，无音轨

Range 请求 `Range: bytes=0-1023` 的结果：

- HTTP 206
- 1,024 bytes
- `Content-Range: bytes 0-1023/1684544`
- `Cache-Control: private, no-store`
- SHA-256：`a481f9c146318539d9175f98c2bc646c4443e56b658f468a50ea8dbc13bb0108`

## 4. 计费验证

### 4.1 对客户的人民币价格

| 分辨率 | 无输入视频 | 含输入视频 |
| --- | ---: | ---: |
| 720p | ¥46 / 百万 tokens | ¥28 / 百万 tokens |
| 1080p | ¥51 / 百万 tokens | ¥31 / 百万 tokens |
| 4K | ¥26 / 百万 tokens（计划价） | ¥16 / 百万 tokens（计划价） |

4K 尚未完成供应商渠道开通和端到端验证，因此没有加入公开 `resolution` 枚举，客户当前只能提交 720p 或 1080p。

通用公式：

```text
人民币费用 = 最终 total_tokens ÷ 1,000,000 × 对应规格的人民币单价
```

本次输入只有参考图片，没有输入视频，因此使用“720p、无输入视频”单价 ¥46 / 百万 tokens。参考图片不会触发“含输入视频”价格。

### 4.2 预扣与最终结算

预估：

```text
估算 tokens = 86,400
估算人民币费用 = 86,400 ÷ 1,000,000 × 46
                 = ¥3.974400
预扣 quota = round(3.974400 ÷ 7.3 × 500,000)
            = 272,219
```

供应商最终账单：

```json
{
  "id": 2347,
  "resolution": "720p",
  "ratio": "16:9",
  "dur": 4,
  "expense_time": "2026-07-18 22:53:46",
  "total_tokens": 87277,
  "price": "46.000000",
  "original_price": "4.0",
  "discount": "1.00",
  "amount_paid": "4.0"
}
```

最终结算：

```text
最终人民币费用 = 87,277 ÷ 1,000,000 × 46
                 = ¥4.014742
最终 quota = round(4.014742 ÷ 7.3 × 500,000)
           = 274,982
差额 quota = 274,982 - 272,219
           = +2,763
```

任务结算状态为 `settled`，系统按最终账单补扣 2,763 quota。对外报价和客户费用均使用人民币；`7.3` 只参与当前 new-api 内部美元锚定 quota 单位换算，不改变人民币价格公式。若后续把账户 quota 直接锚定人民币，可去除该内部换算层，但必须同步迁移充值、余额展示、日志和历史账务口径。

### 4.3 内部账单查询窗口和时区

任务成功后，后台使用供应商私有接口：

```text
POST https://api.laomandi.com/asset/SdToolApi/ListSplitBillDetail
```

本次最终匹配任务 2347 的查询窗口为：

```json
{
  "expense_date_start": "2026-07-18 21:53:46",
  "expense_date_end": "2026-07-18 23:58:00",
  "page": 0,
  "size": 1000
}
```

窗口不是严格的“创建时刻到完成时刻”，而是在任务提交时间前和当前时间后各留出一小时余量，以覆盖供应商账单落库延迟。供应商 `expense_time` 和查询参数统一按 Asia/Shanghai（UTC+8）构造，避免把 UTC 时间直接发送给使用北京时间的供应商接口。

## 5. 鉴权与错误行为

- 四个视频接口缺少或使用无效 Token：HTTP 401，响应为 `new_api_error` 格式。
- 五个真人认证/素材接口缺少 API Key：HTTP 401，`error.code=token_not_provided`；无效 Key 使用 `token_invalid`。
- 真人认证和素材直通接口的业务失败可能返回 HTTP 200；必须检查 `state/data/error`。
- `CreateAsset` 的已知供应商业务错误会转为 HTTP 400，并保留具体业务错误码和消息。
- `GET /v1/videos/{task_id}/content` 只允许当前 API Key 所属账号读取自己的成功任务，并固定返回 `Cache-Control: private, no-store`。
- 供应商 `Set-Cookie` 响应头由素材代理剥离，不会泄漏 PHP 会话标识给客户。

## 6. 最终结论与未覆盖项

公网 IP HTTPS 入口已能完整承载 9 个公开 Seedance API。客户可使用素材库图片创建视频，轮询国内格式或 OpenAI 兼容格式的任务状态，并通过网关代理完整或分段下载结果。

当前唯一未完成的成功路径是必须由真人交互的活体认证结果获取。若需要把该分支也标记为端到端通过，测试人员需在创建会话后的 120 秒内打开 H5Link、完成人工认证，并立即用回调返回的 BytedToken 调用 `GetVisualValidateResult`；该步骤不会生成视频费用。
