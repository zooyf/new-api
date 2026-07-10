# Enterprise Policy Hub 旁路组织与 Key 管理方案

## 1. 背景与目标

客户希望不要让员工自己注册 new-api 用户，也不要让管理员批量创建员工用户后让员工自行生成 API Key。客户希望由管理员统一创建和管理 Key，并按公司组织架构控制部门、项目、成本中心的模型权限、用量和预算。

本方案采用旁路服务实现，不改变 new-api 的请求、路由和计费链路。为保证旁路服务直接更新 `tokens` 后与 Redis 缓存一致，只在 `model/token.go`、`model/token_cache.go` 增加两个窄接口；异步任务初始消费日志额外写入已生成的公开 `task_id`，让 Hub 能区分预扣与最终结算。业务请求不会经过 Policy Hub。new-api 继续负责现有能力：

- API Key 鉴权
- `token.group` 分组路由
- `token.model_limits` 模型限制
- 渠道选择与模型映射
- 调用上游供应商
- 计费、日志和用量明细

旁路服务负责企业管理能力：

- 多层组织架构
- 企业 Policy
- 管理员集中创建和管理企业 Key
- Policy 编译并同步到 new-api token
- 从 new-api 日志回收用量并归集到组织树
- 预算、告警、超限禁用
- 企业管理后台和组织级权限控制

第一期不做 RelayMode 级控制，也不引入请求前 Policy Proxy。控制重点是组织、Key、模型、group、预算和用量归集。

## 2. 核心结论

最终架构是：

```text
浏览器管理员
  -> /enterprise/
  -> Enterprise Policy Hub 管理后台
  -> 复用 new-api 管理员身份做登录校验
  -> Policy Hub 自己做组织级 RBAC

业务调用方 / 员工 / 内部系统
  -> 直接调用 new-api 原有 /v1/* API
  -> Authorization: Bearer 企业 Key
  -> new-api 原样执行 TokenAuth / Distribute / Billing / Logs

Policy Hub 后台任务
  -> 同步企业 Key 到 new-api tokens
  -> 拉取 new-api logs / quota_data
  -> 按 token_id 归集到部门 / 项目 / 成本中心
```

new-api 侧看到的仍然是：

```text
token.group
token.model_limits_enabled
token.model_limits
token.status
token.remain_quota
```

Policy Hub 侧看到的是：

```text
组织架构
Policy
企业 Key
部门预算
项目预算
成本中心
同步状态
用量报表
```

## 3. new-api 改动边界

第一期明确不改核心链路：

- 不改 `middleware/auth.go`
- 不改 `middleware/distributor.go`
- 不改渠道选择逻辑
- 不改计费逻辑
- 不改使用日志逻辑
- 不改 new-api 前端菜单

仅保留三个稳定补丁点：

- `model.InvalidateTokenCache`：删除企业 Key 时撤销 Redis token 缓存。
- `model.UpdateTokenCacheAfterExternalWrite`：Policy Hub 更新 token 元数据或额度上限时，以 Redis Lua 原子更新缓存；保留 new-api 批量额度更新尚未落库的消费差值，避免从旧数据库值重新补充额度。
- `service.LogTaskConsumption`：把 `RelayInfo.PublicTaskID` 写入异步预扣日志的 `other.task_id`。Hub 将该额度记为 pending，任务完成、失败退款或差额结算后才纳入预算阻断判断。

这组补丁不增加新的网络调用或数据库写入，后续合并社区更新时只需维护 `model/token.go`、`model/token_cache.go` 和 `service/task_billing.go` 三个稳定点。

如果希望在 new-api 后台菜单里增加“企业策略中心”外链，那属于可选前端小改动，不属于第一期必要范围。

## 4. new-api 现有能力映射

基于当前代码，旁路服务可以直接复用这些能力：

| new-api 能力 | 代码/数据对应 | 旁路服务如何使用 |
|---|---|---|
| API Key | `tokens` 表 | Policy Hub 创建/更新 new-api token |
| Key 所属用户 | `tokens.user_id` | 使用部门服务账号承载 token |
| Key 使用分组 | `tokens.group` | 由 Policy 的 `default_group` 同步 |
| Key 模型白名单 | `tokens.model_limits_enabled`、`tokens.model_limits` | 由 Policy 的有效模型列表同步 |
| Key 启停 | `tokens.status` | 超预算或禁用时同步 |
| Key 额度 | `tokens.remain_quota`、`tokens.unlimited_quota` | 可选同步预算额度 |
| 用户分组 | `users.group` | 部门服务账号所属基础 group |
| 渠道分组 | `channels.group` | new-api 管理员继续维护底层渠道池 |
| 模型渠道能力 | `abilities.group + model + channel_id` | new-api 现有渠道能力表继续生效 |
| 使用日志 | `logs.token_id`、`logs.group`、`logs.model_name`、`logs.quota` | Policy Hub 按 `token_id` 回收归集 |

当前 new-api 的 Key 不支持 RelayMode 级控制，因此本方案不依赖该能力。

## 5. 管理入口

推荐同域名独立入口：

```text
https://llm.ai.nexus-reach.com/enterprise/
```

Nginx 路由示例：

```nginx
location /enterprise/ {
    proxy_pass http://127.0.0.1:3100/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

location / {
    proxy_pass http://127.0.0.1:3000;
}
```

日常管理都在 Policy Hub 中完成。new-api 原后台仍用于底层供应商、渠道、模型价格、分组和倍率配置。

## 6. 权限设计

### 6.1 身份认证

Policy Hub 不单独维护登录账号密码，第一期复用 new-api 管理员身份。

推荐流程：

```text
管理员已登录 new-api
  -> 打开 /enterprise/
  -> 浏览器携带 new-api session cookie
  -> Policy Hub 后端调用 new-api 身份校验接口
  -> 校验当前用户是否为 admin/root
  -> 通过后创建 Policy Hub 自己的短 session
```

也可以支持 new-api 用户 AccessToken：

```text
Authorization: Bearer NEW_API_USER_ACCESS_TOKEN
New-Api-User: <user_id>
```

new-api 现有 `AdminAuth` 逻辑要求 `role >= 10`。角色含义：

```text
RoleCommonUser = 1
RoleAdminUser  = 10
RoleRootUser   = 100
```

Policy Hub 调用 new-api 校验身份时要注意：new-api 某些鉴权失败会返回 HTTP 200 但 `success=false`，所以不能只看 HTTP status。

### 6.2 授权边界

new-api 只负责回答：

```text
这个登录者是不是 new-api 管理员 / 超级管理员？
```

Policy Hub 自己负责回答：

```text
这个管理员能不能管理某个组织节点、Policy、Key、预算、报表？
```

### 6.3 Policy Hub 角色

| 角色 | 权限 |
|---|---|
| `hub_super_admin` | 管理所有组织、Policy、Key、预算、同步配置 |
| `hub_org_admin` | 管理指定组织节点及下级节点 |
| `hub_key_admin` | 创建/禁用 Key，不能改全局 Policy |
| `hub_finance_admin` | 查看预算、用量和成本中心报表 |
| `hub_auditor` | 只读审计日志和报表 |

`hub_super_admin` 可以默认映射 new-api root，也可以由 Policy Hub 显式授权。

### 6.4 审计要求

以下操作必须记录审计日志：

- 登录成功/失败
- 创建、启用、禁用、删除企业 Key
- 查看完整 Key
- 修改组织节点
- 修改 Policy
- 修改模型列表
- 修改预算
- 修改成本中心
- 手动触发同步
- 导出用量明细

审计字段：

```text
admin_newapi_user_id
admin_username
admin_role
hub_role
action
target_type
target_id
before_json
after_json
ip
user_agent
created_at
```

## 7. 组织架构设计

### 7.1 组织树

Policy Hub 管理多层组织：

```text
公司
  ├─ 事业部
  │   ├─ 部门
  │   │   ├─ 团队
  │   │   └─ 项目
  └─ 成本中心
```

组织节点字段建议：

```text
id
parent_id
path
name
code
type               company / business_unit / department / team / project / cost_center
status
owner_admin_id
default_policy_id
default_group
created_at
updated_at
```

建议额外维护闭包表或 materialized path，加速查询祖先链和下级节点。

### 7.2 组织节点与 new-api group

组织节点不是 new-api group，但可以映射到 group：

```text
销售部 org_unit
  -> default_group = dept_sales

研发部 org_unit
  -> default_group = dept_rd
```

多个组织节点可以共享同一个 group：

```text
销售一部 -> sales_standard
销售二部 -> sales_standard
```

也可以每个部门独立 group：

```text
销售一部 -> sales_team_1
销售二部 -> sales_team_2
```

选择原则：

- 如果要共享渠道池和价格，使用同一个 group。
- 如果要隔离渠道、模型路由或价格，使用不同 group。

## 8. Policy 设计

Policy 不等于 group。Policy 是企业策略，group 是 new-api 路由/计费分组。

Policy 字段建议：

```text
id
name
description
default_group
allowed_models
denied_models
monthly_budget
daily_budget
currency
key_default_quota
inherit_mode
status
created_at
updated_at
```

Policy 示例：

```json
{
  "name": "sales-basic",
  "default_group": "dept_sales",
  "allowed_models": [
    "gpt-4o-mini",
    "claude-haiku",
    "doubao-lite"
  ],
  "denied_models": [],
  "monthly_budget": 500,
  "currency": "USD",
  "key_default_quota": 100
}
```

### 8.1 继承规则

一个 Key 绑定到组织树的某个节点时，有效策略从根节点往下合并：

```text
公司 Policy
  -> 事业部 Policy
  -> 部门 Policy
  -> 项目/团队 Policy
  -> Key 自身覆盖
  -> effective_policy
```

推荐合并规则：

| 配置项 | 合并规则 |
|---|---|
| `allowed_models` | 默认取交集，子级只能收窄 |
| `denied_models` | deny 向下继承，优先级高于 allow |
| `default_group` | 子级可覆盖父级 |
| `monthly_budget` | 子级预算不能超过父级预算约束 |
| `daily_budget` | 子级预算不能超过父级预算约束 |
| `key_default_quota` | 子级可收窄或覆盖 |

如果需要允许子级扩展模型，必须由上级显式开启 `allow_child_expand_models`，第一期不建议开放。

## 9. 企业 Key 管理

客户不希望员工注册，因此采用管理员集中创建企业 Key。

Policy Hub 中的企业 Key：

```text
id
name
org_unit_id
project_id
cost_center_id
policy_id
newapi_user_id
newapi_token_id
newapi_token_name
key_fingerprint
status
environment      prod / test / dev
purpose
contact
created_by
created_at
updated_at
```

new-api 中对应 token：

```text
tokens.name = enterprise_key.name
tokens.user_id = 部门服务账号或企业服务账号
tokens.group = effective_policy.default_group
tokens.model_limits_enabled = true
tokens.model_limits = effective_policy.allowed_models 逗号分隔
tokens.status = enabled / disabled
tokens.remain_quota = 可选同步额度
```

完整 Key 只在创建时展示一次。之后只显示指纹：

```text
sk-abc...xyz
```

## 10. 部门服务账号策略

为了不让员工登录 new-api，建议在 new-api 里只创建少量服务账号：

```text
dept-sales-service
dept-rd-service
dept-marketing-service
```

Policy Hub 创建企业 Key 时，把 new-api token 挂到对应服务账号下。

可选策略：

| 策略 | 说明 |
|---|---|
| 每部门一个服务账号 | 用量和 token 列表天然按部门隔离 |
| 全企业一个服务账号 | 实现最简单，但 new-api 里按用户看不出部门 |
| 每项目一个服务账号 | 项目隔离更清晰，但账号较多 |

推荐第一期：每部门一个服务账号。

## 11. 同步机制

### 11.1 同步方向

```text
Policy Hub -> new-api
```

同步内容：

| Policy Hub effective_policy | new-api |
|---|---|
| `default_group` | `tokens.group` |
| `allowed_models` | `tokens.model_limits` |
| 模型限制启用 | `tokens.model_limits_enabled = true` |
| Key 状态 | `tokens.status` |
| Key 额度 | `tokens.remain_quota` |
| 部门服务账号 | `users` |

### 11.2 同步方式

优先级：

1. 直接连接 new-api 主库，使用 GORM/SQL 更新 `users`、`tokens`。
2. 或调用 new-api 管理 API。

第一期更建议直接连接数据库，因为需要批量同步、幂等和恢复，对旁路服务更可控。

但必须遵守：

- 不打印 new-api DB DSN
- 不打印完整 Key
- 同步操作写 audit log
- 同步任务幂等
- 同步失败可重试

### 11.3 同步状态

维护 `newapi_sync_jobs`：

```text
id
entity_type        enterprise_key / policy / org_unit
entity_id
operation          create_token / update_token / disable_token
status             pending / running / succeeded / failed
error_message
retry_count
created_at
updated_at
```

维护 `enterprise_keys.sync_status`：

```text
pending
synced
failed
disabled
```

## 12. 用量归集

new-api 继续写原有日志。Policy Hub 周期拉取：

```text
logs
quota_data
tasks（如需要异步任务状态）
```

最关键关联键：

```text
logs.token_id -> enterprise_keys.newapi_token_id
```

归集流程：

```text
读取 new-api 增量日志
  -> 按 token_id 找 enterprise_key
  -> 找 org_unit / project / cost_center
  -> 计算金额和用量
  -> 写 organization_usage_ledger
  -> 更新预算使用量
```

报表维度：

| 维度 | 说明 |
|---|---|
| 组织节点 | 公司、事业部、部门、团队 |
| 项目 | 项目用量 |
| 成本中心 | 财务归属 |
| 企业 Key | 某个 Key 的使用 |
| 模型 | 模型维度消耗 |
| 渠道 | 实际供应商/渠道消耗 |
| 时间 | 日、周、月 |

## 13. 预算控制

第一期采用准实时预算控制：

```text
定时拉取 new-api 日志
  -> 更新组织/项目/key 用量
  -> 判断是否超过预算
  -> 超预算则同步禁用 token 或把 remain_quota 设置为 0
```

特点：

- 不改 new-api
- 有同步间隔导致的延迟
- 适合企业内部用量治理

预算层级：

```text
公司预算
  -> 事业部预算
  -> 部门预算
  -> 项目预算
  -> Key 预算
```

一次调用归集时要累计到祖先链：

```text
Key +10
项目 +10
部门 +10
事业部 +10
公司 +10
```

如果任意层级超限：

```text
标记 budget_exceeded
触发告警
禁用相关 Key 或阻止继续发放额度
```

强实时预算拦截不在第一期范围。如果以后要求严格实时，需要考虑：

- 改 new-api 增加策略 hook；或
- 引入前置 Policy Proxy；或
- 把组织预算映射成 new-api token quota 并高频同步。

## 14. 管理后台页面

Policy Hub 提供独立管理后台。

页面建议：

| 页面 | 功能 |
|---|---|
| 组织架构 | 树形管理公司/事业部/部门/团队/项目/成本中心 |
| Policy 管理 | 配置 group、允许模型、预算、继承策略 |
| 企业 Key 管理 | 创建、查看、禁用、轮换、绑定组织和 Policy |
| 部门服务账号 | 映射 new-api 服务账号 |
| 同步状态 | 查看 token 同步结果和错误 |
| 用量报表 | 按组织、项目、Key、模型、渠道统计 |
| 预算管理 | 月预算、日预算、超限状态、告警 |
| 审计日志 | 查看所有管理操作 |
| 系统设置 | new-api 连接、日志拉取周期、安全配置 |

## 15. API 草案

### 15.1 身份

```text
GET /enterprise/api/auth/me
POST /enterprise/api/auth/logout
```

`/auth/me` 后端调用 new-api 校验当前管理员身份，并返回 Policy Hub 角色和组织范围。

### 15.2 组织

```text
GET    /enterprise/api/org-units
POST   /enterprise/api/org-units
PUT    /enterprise/api/org-units/{id}
DELETE /enterprise/api/org-units/{id}
```

### 15.3 Policy

```text
GET    /enterprise/api/policies
POST   /enterprise/api/policies
PUT    /enterprise/api/policies/{id}
DELETE /enterprise/api/policies/{id}
POST   /enterprise/api/policies/{id}/preview-effective
```

### 15.4 企业 Key

```text
GET    /enterprise/api/keys
POST   /enterprise/api/keys
GET    /enterprise/api/keys/{id}
PUT    /enterprise/api/keys/{id}
POST   /enterprise/api/keys/{id}/disable
POST   /enterprise/api/keys/{id}/enable
POST   /enterprise/api/keys/{id}/rotate
POST   /enterprise/api/keys/{id}/sync
```

### 15.5 用量与预算

```text
GET /enterprise/api/usage/summary
GET /enterprise/api/usage/details
GET /enterprise/api/budgets
PUT /enterprise/api/budgets/{id}
POST /enterprise/api/budgets/{id}/reset
```

### 15.6 审计

```text
GET /enterprise/api/audit-logs
```

## 16. 数据库表草案

核心表：

```text
eph_org_units
eph_org_unit_closure
eph_hub_admin_bindings
eph_policies
eph_enterprise_keys
eph_budget_accounts
eph_budget_transactions
eph_organization_usage_ledger
eph_newapi_sync_jobs
eph_audit_logs
eph_settings
```

### 16.1 `eph_hub_admin_bindings`

```text
id
newapi_user_id
newapi_username
hub_role
scope_org_unit_id
status
created_at
updated_at
```

### 16.2 `eph_organization_usage_ledger`

```text
id
newapi_log_id
newapi_token_id
enterprise_key_id
org_unit_id
project_id
cost_center_id
model_name
channel_id
use_group
quota
amount
currency
created_at
```

### 16.3 `eph_budget_transactions`

```text
id
budget_account_id
source_type       newapi_log / manual_adjustment / reset
source_id
amount
direction         consume / refund / adjustment
created_at
```

## 17. 部署方案

建议独立容器：

```text
new-api              :3000
enterprise-policy-hub :3100
nginx
mysql/postgres/sqlite
redis 可选
```

Compose 服务示例：

```yaml
services:
  enterprise-policy-hub:
    image: same-new-api-image
    restart: unless-stopped
    entrypoint: ["/enterprise-policy-hub"]
    environment:
      EPH_PORT: "3100"
      EPH_BASE_PATH: "/enterprise"
      EPH_NEWAPI_BASE_URL: "http://new-api:3000"
      EPH_BOOTSTRAP_ADMIN_IDS: "${EPH_BOOTSTRAP_ADMIN_IDS}"
      EPH_LOG_SYNC_INTERVAL_SECONDS: "60"
      SQL_DSN: "${SQL_DSN}"
      LOG_SQL_DSN: "${LOG_SQL_DSN}"
```

敏感信息放入独立 env：

```text
/opt/new-api/enterprise-policy-hub.env
chmod 600
```

## 18. 安全要求

第一期必须做到：

- 管理 API 全部鉴权
- 复用 new-api admin/root 身份
- Policy Hub 自己做组织级 RBAC
- 完整 Key 只展示一次
- 密钥、DSN 不进日志
- 管理操作写审计
- CSRF 防护
- CORS 限制同域
- HTTPS
- 登录失败/敏感接口限流
- 支持 IP allowlist 或 VPN 访问

## 19. 实施计划

### Phase 1：旁路基础版

目标：不改 new-api，实现部门 Key 管理、模型控制、用量归集。

任务：

1. 搭建 Enterprise Policy Hub 服务。
2. 实现 new-api 管理员身份校验。
3. 实现 Policy Hub RBAC。
4. 实现组织树。
5. 实现 Policy：`default_group + allowed_models + budget`。
6. 实现企业 Key 创建。
7. 同步 new-api token：`group + model_limits + status + quota`。
8. 拉取 new-api 日志。
9. 按 `token_id` 归集用量。
10. 做部门/项目/Key 报表。
11. 超预算自动禁用 Key。
12. 审计日志。

验收：

- 管理员可通过 new-api 身份进入 `/enterprise/`。
- 非管理员无法进入。
- 创建企业 Key 后 new-api 中出现对应 token。
- token 的 group 和 model_limits 与 Policy 一致。
- 使用该 Key 调用 new-api 成功。
- 调用日志能在 Policy Hub 中归属到部门。
- 超预算后 token 被禁用。

### Phase 2：组织继承与预算增强

任务：

1. 支持组织树 Policy 继承。
2. 支持预算祖先链归集。
3. 支持预算告警。
4. 支持 Key 额度周期重置。
5. 支持成本中心报表。
6. 支持模型维度和渠道维度分析。

### Phase 3：成本与收入增强

任务：

1. 对接 TokenOperation。
2. 区分内部收入和上游成本。
3. 支持供应商成本归集。
4. 支持毛利报表。
5. 支持账期对账。

## 20. 风险与取舍

| 风险 | 说明 | 缓解 |
|---|---|---|
| 预算不是强实时 | 不改 new-api 时只能准实时禁用 | 缩短同步周期，或后续增加 hook/proxy |
| 直接写 new-api DB 有耦合 | 需要跟随 tokens 表结构 | 只写稳定字段，封装同步层 |
| new-api 管理员身份校验依赖现有接口 | 接口变化会影响 Hub 登录 | 封装 Auth Provider，保留 fallback |
| group 配置仍需在 new-api 管理 | 旁路服务不接管渠道池 | 文档和 UI 中明确职责边界 |
| Key 分发仍有泄露风险 | 员工拿到 Key 后可传播 | Key 指纹、IP 限制、周期轮换、审计 |

## 21. 最终执行口径

这套方案的核心是：

```text
Policy Hub 管企业语义。
new-api 管网关执行。
Policy Hub 把企业语义编译成 new-api 已有 token 配置。
Policy Hub 再把 new-api 用量日志还原成企业组织报表。
```

第一期不改 new-api 核心请求链路，不做 RelayMode 控制，不引入请求前代理。仅增加旁路写 token 所需的两个缓存一致性接口。这样既不增加模型 API 调用时延，也便于生产环境持续跟随社区 new-api 更新。

## 22. 端到端流程

### 22.1 管理员进入 Policy Hub

```mermaid
flowchart TD
    A["管理员打开 /enterprise/"] --> B["浏览器携带 new-api session cookie 或 AccessToken"]
    B --> C["Policy Hub 调用 new-api 身份校验"]
    C --> D{"new-api role >= Admin?"}
    D -- "否" --> E["拒绝访问"]
    D -- "是" --> F["读取 Hub Admin Binding"]
    F --> G{"是否有 Hub 权限或是 root/bootstrap?"}
    G -- "否" --> H["拒绝访问"]
    G -- "是" --> I["返回 Hub 角色、组织范围、菜单权限"]
```

这里 new-api 只提供身份可信来源，Policy Hub 自己决定这个管理员能管理哪些组织、Key、预算和报表。

### 22.2 创建企业 Key

```mermaid
flowchart TD
    A["Hub 管理员选择组织节点"] --> B["选择或继承 Policy"]
    B --> C["填写 Key 名称、用途、联系人、环境"]
    C --> D["Policy Hub 计算 effective_policy"]
    D --> E["生成 new-api token"]
    E --> F["写入 tokens.user_id / group / model_limits / status / quota"]
    F --> G["保存 enterprise_key 与 newapi_token_id 映射"]
    G --> H["只展示一次完整 sk-... Key"]
    H --> I["写审计日志"]
```

核心原则是：客户看到的是企业 Key，new-api 实际执行的是普通 token。

### 22.3 客户实际调用

```mermaid
flowchart TD
    A["客户系统携带企业 Key 调用 /v1/*"] --> B["new-api TokenAuth"]
    B --> C["读取 tokens.group 和 tokens.model_limits"]
    C --> D["按 group + model 选择可用 channel"]
    D --> E["转发到上游模型供应商"]
    E --> F["new-api 完成计费并写 logs"]
    F --> G["Policy Hub 后台稍后拉取 logs"]
```

第一期没有请求前旁路代理，所以不会增加大模型 API 调用链路延迟。

### 22.4 用量归集与预算禁用

```mermaid
flowchart TD
    A["定时任务读取 last_newapi_log_id"] --> B["拉取增量 logs"]
    B --> C["按 logs.token_id 匹配 enterprise_key"]
    C --> D["找到 org_unit / project / cost_center"]
    D --> E["写 organization_usage_ledger"]
    E --> F["更新 Key / 项目 / 部门 / 公司预算交易"]
    F --> G{"是否超预算?"}
    G -- "否" --> H["更新 last_newapi_log_id"]
    G -- "是" --> I["禁用相关 enterprise_key"]
    I --> J["同步 tokens.status = disabled"]
    J --> H
```

这是一种准实时控制。它不保证某一次调用在预算边界处被强拦截，但能在同步周期内自动收口。

## 23. 需要新增或调整的文件

第一期应尽量把改动集中在一小组稳定文件中，减少后续跟随社区 new-api 更新的维护成本。

### 23.1 新增后端服务

```text
cmd/enterprise-policy-hub/main.go
pkg/enterprisepolicyhub/config.go
pkg/enterprisepolicyhub/models.go
pkg/enterprisepolicyhub/app.go
pkg/enterprisepolicyhub/auth.go        可选，若 app.go 过大再拆
pkg/enterprisepolicyhub/sync.go        可选，若同步逻辑需要单测
pkg/enterprisepolicyhub/usage.go       可选，若用量归集逻辑需要单测
```

建议第一版先保持直接、清晰，避免过早拆太多包。只有当鉴权、同步、用量归集已经变复杂时，再拆成单独文件。

### 23.2 Docker 与部署

```text
Dockerfile
scripts/deploy-nexus-sg.ps1
scripts/deploy-enterprise-policy-hub-nexus-sg.ps1
deploy/enterprise-policy-hub.compose.override.yml   可选
docs/enterprise-policy-hub-plan.md
```

Dockerfile 在现有镜像里额外构建 `/enterprise-policy-hub` 二进制。生产 Compose 以同一个镜像启动独立容器：

```yaml
enterprise-policy-hub:
  image: same-new-api-image
  entrypoint: ["/enterprise-policy-hub"]
  env_file:
    - /opt/new-api/.env
    - /opt/new-api/enterprise-policy-hub.env
  ports: []
```

### 23.3 Nginx

只新增 `/enterprise/` location，不影响 `/v1/*`、`/api/*`、`/apidocs/` 等现有入口：

```nginx
location /enterprise/ {
    proxy_pass http://127.0.0.1:3100/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

上线前必须备份 Nginx 配置，执行 `nginx -t`，通过后再 reload。

## 24. 环境变量

建议使用 `EPH_` 前缀，避免和 new-api 原有变量混淆。

| 变量 | 默认值 | 说明 |
|---|---:|---|
| `EPH_PORT` | `3100` | Policy Hub 监听端口 |
| `EPH_BASE_PATH` | `/enterprise` | 对外挂载路径 |
| `EPH_NEWAPI_BASE_URL` | 空 | 如果填写，则通过 HTTP 调 new-api `/api/user/self` 校验管理员身份 |
| `EPH_AUTH_TIMEOUT_SECONDS` | `10` | 调 new-api 鉴权接口超时 |
| `EPH_BOOTSTRAP_ADMIN_IDS` | 空 | 首批 Hub 超级管理员的 new-api user id，逗号分隔 |
| `EPH_ALLOW_ANY_NEWAPI_ADMIN` | `false` | 是否允许任意 new-api admin 自动成为 Hub super admin，生产建议 false |
| `EPH_LOG_SYNC_INTERVAL_SECONDS` | `60` | 后台拉取 new-api logs 的周期 |
| `EPH_DISABLE_BACKGROUND_SYNC` | `false` | 是否关闭后台同步，仅允许手动触发 |
| `SQL_DSN` | 复用 new-api | 主库连接。Hub 表和 new-api 主表在同库时最简单 |
| `LOG_SQL_DSN` | 复用 new-api | 日志库连接；如果为空则使用主库 |

生产建议：

```text
EPH_ALLOW_ANY_NEWAPI_ADMIN=false
EPH_BOOTSTRAP_ADMIN_IDS=<root_user_id>
EPH_LOG_SYNC_INTERVAL_SECONDS=60
```

## 25. 数据表命名与兼容性

为了避免和 new-api 未来社区版本新增表名冲突，Policy Hub 表建议统一加前缀：

```text
eph_org_units
eph_org_unit_closure
eph_hub_admin_bindings
eph_policies
eph_enterprise_keys
eph_budget_accounts
eph_budget_transactions
eph_organization_usage_ledger
eph_newapi_sync_jobs
eph_audit_logs
eph_settings
```

数据库实现必须同时兼容 SQLite、MySQL 和 PostgreSQL：

- 优先使用 GORM。
- 不依赖数据库私有 JSON 类型。
- 金额、模型列表、扩展配置优先用 `TEXT` 存储 JSON 或逗号分隔字符串。
- 时间字段沿用项目习惯使用 Unix 秒，减少跨库时间类型差异。
- 所有同步任务必须幂等，重复执行不能重复扣预算或重复创建 token。

## 26. 第一阶段执行清单

### 26.1 服务骨架

1. 新增 `cmd/enterprise-policy-hub/main.go`。
2. 初始化 `.env`、日志、主库、日志库。
3. 设置 `common.IsMasterNode=false`，不启动 new-api 主服务、副作用任务和计费轮询。
4. 执行 `enterprisepolicyhub.Migrate(model.DB)`。
5. 启动 Gin HTTP 服务，提供 `GET /healthz`。

### 26.2 身份与权限

1. 实现 `GET /enterprise/api/auth/me`。
2. 支持通过 cookie 调 `EPH_NEWAPI_BASE_URL/api/user/self` 校验。
3. 支持通过 `Authorization + New-Api-User` 校验 new-api AccessToken。
4. 要求 new-api role 至少为 admin。
5. root 或 bootstrap admin 可进入 Hub。
6. 其他管理员必须在 `eph_hub_admin_bindings` 中显式授权。
7. 所有写操作检查 Hub RBAC。

### 26.3 组织与 Policy

1. 实现组织树 CRUD。
2. 创建组织节点时维护 path 和 closure。
3. 实现 Policy CRUD。
4. 实现 effective_policy 计算：
   - 从根到当前组织节点合并。
   - `allowed_models` 默认取交集。
   - `denied_models` 向下继承并最终剔除。
   - `default_group` 子级覆盖父级。
   - Key 级 Policy 最后覆盖。

### 26.4 企业 Key

1. 创建企业 Key 时生成 new-api token。
2. 保存 `enterprise_key.newapi_token_id`。
3. 保存 Key 指纹，不保存完整 `sk-...`。
4. 只在创建和轮换时返回完整 Key。
5. 支持启用、禁用、轮换、手动同步。
6. 同步失败写 `eph_newapi_sync_jobs`，并保留错误信息。

### 26.5 用量与预算

1. 记录 `last_newapi_log_id`。
2. 定时拉取 new-api 增量 logs。
3. 按 `logs.token_id` 匹配企业 Key。
4. 写入 `eph_organization_usage_ledger`。
5. 对 Key、项目、成本中心、组织祖先链写预算交易。
6. 超预算后禁用企业 Key 并同步到 new-api token。
7. 提供 usage summary/detail API。

### 26.6 管理页面

第一版可以使用独立轻量页面，不必接入 new-api 前端菜单：

```text
/enterprise/
```

页面必须具备：

- 当前登录管理员与 Hub 角色展示。
- 组织树管理。
- Policy 管理。
- 企业 Key 管理。
- 同步状态。
- 用量和预算报表。
- 审计日志。

## 27. 上线步骤

### 27.1 预检查

1. 确认生产数据库连接可用。
2. 确认当前 new-api 服务健康。
3. 确认 root/admin 用户 id。
4. 确认 `/opt/new-api/enterprise-policy-hub.env` 存在且权限为 `600`。
5. 确认 Nginx 配置可备份。
6. 确认 Docker/Compose 有足够磁盘和内存。

### 27.2 构建

1. 本地仓库打包源码 tar 上传服务器，避免依赖服务器访问 GitHub。
2. 服务器使用上传源码构建新镜像。
3. 验证镜像内同时存在：

```text
/new-api
/hwdrama-proxy
/enterprise-policy-hub
```

### 27.3 部署

1. 使用独立脚本只新增或更新 `enterprise-policy-hub` 容器，先不重启 new-api：

```powershell
.\scripts\deploy-enterprise-policy-hub-nexus-sg.ps1 `
  -EnterpriseHubBootstrapAdminIds "<root_user_id>" `
  -AllowDirty `
  -Yes
```

2. 脚本会从当前本机工作区打包源码上传服务器构建镜像，避免依赖服务器访问 GitHub。
3. 脚本会从正在运行的 `new-api` 容器读取 `SQL_DSN`、`LOG_SQL_DSN`、`TZ`，写入 `/opt/new-api/enterprise-policy-hub.env`，权限为 `600`。
4. 脚本会写入 `/opt/new-api/docker-compose.enterprise-policy-hub.override.yml`，并只执行：

```bash
docker compose \
  -f /opt/new-api/docker-compose.yml \
  -f /opt/new-api/docker-compose.enterprise-policy-hub.override.yml \
  up -d --no-deps enterprise-policy-hub
```

5. 检查 `http://127.0.0.1:3100/healthz`。
6. 修改 Nginx 增加 `/enterprise/` include。
7. `nginx -t`。
8. reload Nginx。
9. 通过生产域名访问 `/enterprise/`。

原始手工步骤如下：

1. 只新增 `enterprise-policy-hub` 容器，先不重启 new-api。
2. 检查 `http://127.0.0.1:3100/healthz`。
3. 修改 Nginx 增加 `/enterprise/`。
4. `nginx -t`。
5. reload Nginx。
6. 通过生产域名访问 `/enterprise/`。

### 27.4 回滚

如果 Hub 启动失败：

1. 停止 `enterprise-policy-hub` 容器。
2. 恢复 Nginx 备份配置。
3. reload Nginx。
4. 不影响 new-api 原有调用。

如果 Hub 同步错误：

1. 暂停后台同步：`EPH_DISABLE_BACKGROUND_SYNC=true`。
2. 根据 `eph_newapi_sync_jobs` 和 `eph_audit_logs` 定位变更。
3. 手动恢复对应 token 的 group、model_limits、status 或 quota。

## 28. 验收测试

### 28.1 鉴权测试

| 场景 | 预期 |
|---|---|
| 未登录访问 `/enterprise/api/auth/me` | 401 |
| 普通用户访问 | 403 |
| new-api admin 但未绑定 Hub 权限 | 403，除非开启 bootstrap/allow-any-admin |
| bootstrap root 访问 | 200 |

### 28.2 Key 同步测试

1. 创建组织节点 `测试部门`，设置 `default_group=default`。
2. 创建 Policy，只允许 `gpt-4o-mini`。
3. 创建企业 Key。
4. 检查 new-api `tokens`：

```text
group = default
model_limits_enabled = true
model_limits contains gpt-4o-mini
status = enabled
```

5. 用该 Key 调用允许模型，预期成功。
6. 用该 Key 调用未允许模型，预期被 new-api 拒绝。

### 28.3 用量归集测试

1. 用企业 Key 发起一次成功调用。
2. 手动触发 `/enterprise/api/usage/sync`。
3. 检查 `eph_organization_usage_ledger` 出现记录。
4. 检查 summary API 能按组织、Key、模型展示用量。

### 28.4 预算超限测试

1. 给企业 Key 设置很小预算。
2. 发起调用并同步日志。
3. 预算超限后，企业 Key 状态变为 disabled。
4. new-api 对应 token 状态也变为 disabled。
5. 再用该 Key 调用，预期失败。

## 29. 后续增强方向

第一期完成后，再根据客户需求选择增强：

1. 对接 TokenOperation，把收入、上游成本和毛利统一展示。
2. 增加预算告警通知：邮件、企业微信、飞书、Slack。
3. 增加 Key 生命周期：申请、审批、到期、自动轮换。
4. 增加部门服务账号自动创建。
5. 增加导出：CSV、Excel、按账期报表。
6. 如果客户要求强实时预算，再考虑 new-api hook 或前置 Policy Proxy。
7. 如果客户要求公司 SSO，再接入 OIDC/SAML，但仍保留 new-api admin fallback。

## 30. 维护策略

为了方便长期跟随社区 new-api 更新，维护原则是：

- 企业能力尽量放在 `cmd/enterprise-policy-hub` 和 `pkg/enterprisepolicyhub`。
- 对 new-api 原有 runtime 尽量零侵入。
- 对 new-api 表只写稳定字段：`tokens.group`、`tokens.model_limits_enabled`、`tokens.model_limits`、`tokens.status`、`tokens.remain_quota`。
- 所有 new-api 交互封装在同步层和鉴权层，避免业务代码到处直接访问 new-api 内部细节。
- Hub 表使用 `eph_` 前缀，降低未来表名冲突概率。
- 每次升级社区 new-api 后，只需要重点回归：
  - token 字段是否变化；
  - logs 字段是否变化；
  - admin 身份校验接口是否变化；
  - Dockerfile 构建是否仍包含 Hub 二进制。
