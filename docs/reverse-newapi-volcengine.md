# reverse-newapi-volcengine 设计文档

## 背景

`reverse-newapi-volcengine` 是一个独立的 Go HTTP 服务，用于把下游的火山方舟视频生成任务协议反转到上游 new-api 视频任务协议。

目标端点：

- `POST /v1/video/generations`
- `GET /v1/video/generations/{task_id}`

现有 new-api 主服务中的 `relay/channel/task/doubao/adaptor.go` 已实现正向转换：下游按 new-api 视频任务格式请求，上游按火山 `contents/generations/tasks` 格式请求。本服务实现相反方向：下游按火山任务格式请求，本服务转成 new-api 视频任务格式后请求上游。

## 启动配置

- `NEW_API_BASE_URL`：必填，上游 new-api 服务地址，例如 `https://new-api.example.com`。启动时缺失或格式非法则直接退出。
- `PORT`：可选，本服务监听端口，默认 `3001`。
- `NEW_API_TIMEOUT_SECONDS`：可选，请求上游 new-api 的超时时间，默认 `120` 秒。

服务不会静默切换到其他上游，也不会在 `NEW_API_BASE_URL` 不可用时回退到 mock 或本地伪实现。

## 提交流程

入口：`POST /v1/video/generations`

下游请求体使用火山任务格式，关键字段参考 `relay/channel/task/doubao/adaptor.go` 中的 `requestPayload`：

- `model`
- `content`
- `callback_url`
- `return_last_frame`
- `service_tier`
- `execution_expires_after`
- `generate_audio`
- `draft`
- `tools`
- `safety_identifier`
- `priority`
- `resolution`
- `ratio`
- `duration`
- `frames`
- `seed`
- `camera_fixed`
- `watermark`

转换为上游 new-api 请求体：

```json
{
  "model": "doubao-seedance-1-0-lite-t2v",
  "prompt": "从 content 中提取的文本",
  "duration": 5,
  "metadata": {
    "content": [],
    "resolution": "720p",
    "ratio": "16:9"
  }
}
```

转换规则：

- `model` 原样写入 new-api 顶层 `model`。
- `content` 中所有 `type=text` 或含 `text` 字段的片段按原顺序拼接为 `prompt`，多个文本之间用换行分隔。
- `duration` 如果存在则写入 new-api 顶层 `duration`，同时仍保留在 `metadata` 中，便于 new-api 的 Doubao 适配器还原火山字段。
- 除 `model` 外的火山原始字段完整放入 `metadata`。
- `metadata.content` 保留原始多模态内容；new-api 的 Doubao 适配器会移除其中的 text 内容并把顶层 `prompt` 重新追加为火山 text 内容，从而避免模型字段被 metadata 覆盖。
- 如果缺少 `model` 或无法从 `content` 提取非空 `prompt`，本服务直接返回 `400`，不请求上游。

上游请求：

- 方法：`POST`
- 地址：`${NEW_API_BASE_URL}/v1/video/generations`
- 鉴权：透传下游 `Authorization` 头。
- 请求体：new-api 视频任务格式。

上游成功响应兼容以下 new-api 形态：

- OpenAI Video 对象：`id` 或 `task_id`
- 旧视频响应：`task_id`
- 通用任务响应：`data.task_id`

下游响应统一转为火山提交响应：

```json
{
  "id": "task_xxx"
}
```

如果上游成功响应中无法解析任务 ID，则返回 `502 invalid_response`。

## 查询流程

入口：`GET /v1/video/generations/{task_id}`

上游请求：

- 方法：`GET`
- 地址：`${NEW_API_BASE_URL}/v1/video/generations/{task_id}`
- 鉴权：透传下游 `Authorization` 头。

上游成功响应兼容以下形态：

- new-api 通用任务响应：`{"code":"success","data":{...}}`
- OpenAI Video 对象：`{"id":"...","object":"video",...}`
- 文档中的视频任务响应：`{"task_id":"...","status":"completed",...}`

下游响应统一转为火山任务对象：

```json
{
  "id": "task_xxx",
  "model": "doubao-seedance-1-0-lite-t2v",
  "status": "succeeded",
  "content": {
    "video_url": "https://example.com/video.mp4"
  },
  "error": {
    "code": "",
    "message": ""
  },
  "created_at": 1760000000,
  "updated_at": 1760000010
}
```

状态映射：

| new-api 状态 | 火山状态 |
| --- | --- |
| `NOT_START` / `SUBMITTED` / `QUEUED` / `queued` | `queued` |
| `IN_PROGRESS` / `in_progress` | `processing` |
| `SUCCESS` / `completed` / `succeeded` | `succeeded` |
| `FAILURE` / `failed` | `failed` |
| 其他未知状态 | `processing` |

结果 URL 优先级：

1. new-api 通用任务响应中的 `data.result_url`
2. 历史兼容字段 `data.fail_reason`
3. 原始上游任务数据 `data.data.content.video_url`
4. OpenAI Video 的 `metadata.url`
5. 文档视频响应中的 `url`

## 错误处理

- 本地参数错误返回 `400`，响应体为 `{"error":{"code":"invalid_request","message":"..."}}`。
- 上游网络错误返回 `502 upstream_request_failed`。
- 上游非 2xx 响应按原状态码和响应体透传，避免篡改 new-api 的错误语义。
- 上游 2xx 但响应结构无法转换时返回 `502 invalid_response`。

## 不做项

- 不实现任务轮询、计费、数据库写入或本地任务存储；这些能力仍由上游 new-api 负责。
- 不实现无鉴权降级；下游未带 `Authorization` 时仍请求上游，由上游 new-api 决定是否拒绝。
- 不修改主 new-api 服务的既有路由、适配器、计费和任务状态更新逻辑。

## 生产路径

服务同时支持两组入口：

- 火山官方风格：`POST /api/v3/contents/generations/tasks`
- 火山官方风格：`GET /api/v3/contents/generations/tasks/{task_id}`
- 兼容旧入口：`POST /v1/video/generations`
- 兼容旧入口：`GET /v1/video/generations/{task_id}`

生产 Nginx 应只把火山官方风格路径转发到本服务，其余视频接口继续由主 new-api 服务处理。
