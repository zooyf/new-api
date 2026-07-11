package enterprisepolicyhub

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *App) index(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(indexHTML))
}

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Enterprise Policy Hub</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f6f7f9;
      --panel: #ffffff;
      --line: #dfe3ea;
      --text: #172033;
      --muted: #647084;
      --brand: #0d766e;
      --brand-dark: #075e58;
      --danger: #b42318;
      --code: #f1f5f9;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font: 14px/1.5 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    header {
      position: sticky;
      top: 0;
      z-index: 20;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 16px 24px;
      background: rgba(255, 255, 255, .94);
      border-bottom: 1px solid var(--line);
      backdrop-filter: blur(12px);
    }
    h1, h2, h3 { margin: 0; line-height: 1.2; }
    h1 { font-size: 20px; }
    h2 { font-size: 16px; }
    h3 { font-size: 14px; color: var(--muted); }
    main {
      display: grid;
      grid-template-columns: 240px minmax(0, 1fr);
      min-height: calc(100vh - 65px);
    }
    nav {
      border-right: 1px solid var(--line);
      padding: 18px 12px;
      background: #fbfcfd;
    }
    nav button {
      width: 100%;
      margin: 3px 0;
      padding: 10px 12px;
      border: 0;
      border-radius: 6px;
      background: transparent;
      color: var(--text);
      text-align: left;
      cursor: pointer;
    }
    nav button.active, nav button:hover {
      background: #e7f5f3;
      color: var(--brand-dark);
    }
    section { display: none; padding: 22px; }
    section.active { display: block; }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
      gap: 16px;
      margin-top: 16px;
    }
    .management-grid {
      grid-template-columns: minmax(300px, 380px) minmax(0, 1fr);
      align-items: start;
    }
    .management-grid > .panel { min-width: 0; }
    .section-heading {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
    }
    .section-heading p {
      margin: 5px 0 0;
      color: var(--muted);
      font-size: 12px;
    }
    .org-explorer { margin-top: 16px; }
    .org-filter-bar {
      display: grid;
      grid-template-columns: minmax(240px, 1fr) 180px 160px auto;
      gap: 10px;
      align-items: end;
      margin-bottom: 14px;
    }
    .org-filter-bar label { margin: 0; }
    .org-tree-scroll {
      width: 100%;
      overflow-x: auto;
      border: 1px solid var(--line);
      border-radius: 8px;
    }
    .org-tree-grid {
      min-width: 1120px;
      display: grid;
      grid-template-columns: minmax(310px, 1.7fr) 130px minmax(200px, 1.1fr) 150px minmax(180px, 1fr) 100px 210px;
    }
    .org-tree-header { display: contents; }
    .org-tree-header > div {
      position: sticky;
      top: 0;
      z-index: 3;
      padding: 9px 10px;
      border-bottom: 1px solid var(--line);
      background: #f8fafc;
      color: var(--muted);
      font-size: 12px;
      font-weight: 700;
    }
    .org-tree-row { display: contents; }
    .org-tree-row > div {
      min-width: 0;
      padding: 9px 10px;
      border-bottom: 1px solid var(--line);
      background: var(--panel);
    }
    .org-tree-row:last-child > div { border-bottom: 0; }
    .org-tree-row:hover > div { background: #f8fbfb; }
    .org-tree-row.selected > div { background: #eefaf8; }
    .org-node-cell {
      display: flex;
      align-items: flex-start;
      gap: 7px;
    }
    .org-node-indent {
      flex: 0 0 auto;
      width: calc(var(--org-depth) * 20px);
      min-height: 1px;
    }
    .org-tree-toggle, .org-tree-spacer {
      flex: 0 0 24px;
      width: 24px;
      height: 24px;
    }
    .org-tree-toggle {
      display: inline-grid;
      place-items: center;
      border: 0;
      border-radius: 4px;
      background: transparent;
      color: var(--muted);
      cursor: pointer;
      font-size: 18px;
      line-height: 1;
    }
    .org-tree-toggle:hover { background: #e7f5f3; color: var(--brand-dark); }
    .org-tree-toggle[aria-expanded="true"] { transform: rotate(90deg); }
    .org-node-main { min-width: 0; }
    .org-name-button {
      display: block;
      max-width: 100%;
      padding: 0;
      border: 0;
      background: transparent;
      color: var(--text);
      cursor: pointer;
      font: inherit;
      font-weight: 700;
      text-align: left;
      overflow-wrap: anywhere;
    }
    .org-name-button:hover { color: var(--brand-dark); text-decoration: underline; }
    .org-node-path, .org-cell-source {
      margin-top: 2px;
      color: var(--muted);
      font-size: 11px;
      line-height: 1.35;
    }
    .org-type-badge, .org-source-badge {
      display: inline-flex;
      align-items: center;
      min-height: 22px;
      padding: 2px 7px;
      border: 1px solid var(--line);
      border-radius: 5px;
      background: #f8fafc;
      color: var(--text);
      font-size: 11px;
      font-weight: 600;
    }
    .org-source-badge { margin-left: 5px; color: var(--brand-dark); border-color: #b7ded9; background: #eefaf8; }
    .org-status-dot {
      display: inline-block;
      width: 8px;
      height: 8px;
      margin-right: 6px;
      border-radius: 50%;
      background: #98a2b3;
    }
    .org-status-dot.enabled { background: #079455; }
    .org-tree-empty { padding: 28px; color: var(--muted); text-align: center; }
    .org-drawer-backdrop {
      position: fixed;
      inset: 0;
      z-index: 60;
      display: none;
      justify-content: flex-end;
      background: rgba(15, 23, 42, .36);
    }
    .org-drawer-backdrop.open { display: flex; }
    .org-drawer {
      width: min(560px, 100vw);
      height: 100%;
      overflow-y: auto;
      background: var(--panel);
      box-shadow: -12px 0 28px rgba(15, 23, 42, .16);
    }
    .org-drawer-header {
      position: sticky;
      top: 0;
      z-index: 2;
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 12px;
      padding: 18px 20px;
      border-bottom: 1px solid var(--line);
      background: rgba(255, 255, 255, .96);
    }
    .org-drawer-close {
      display: inline-grid;
      place-items: center;
      flex: 0 0 34px;
      width: 34px;
      height: 34px;
      border: 0;
      border-radius: 6px;
      background: #eef2f7;
      color: var(--text);
      cursor: pointer;
      font-size: 20px;
    }
    .org-breadcrumb { margin-top: 5px; color: var(--muted); font-size: 12px; }
    .org-drawer-body { padding: 18px 20px 28px; }
    .org-effective-summary {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 0 18px;
      margin-bottom: 18px;
      border-top: 1px solid var(--line);
      border-bottom: 1px solid var(--line);
    }
    .org-effective-item { padding: 11px 0; }
    .org-effective-item .label { color: var(--muted); font-size: 11px; }
    .org-effective-item .value { margin-top: 3px; font-weight: 600; overflow-wrap: anywhere; }
    .org-form-actions {
      position: sticky;
      bottom: 0;
      margin: 18px -20px -28px;
      padding: 14px 20px;
      border-top: 1px solid var(--line);
      background: rgba(255, 255, 255, .96);
    }
    .report-grid { grid-template-columns: 1fr; }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 16px;
      min-width: 0;
    }
    .toolbar {
      display: flex;
      align-items: center;
      gap: 10px;
      flex-wrap: wrap;
      margin: 12px 0;
    }
    .toolbar > label {
      min-width: 180px;
      flex: 1 1 180px;
      margin: 0;
    }
    header .toolbar {
      flex-wrap: nowrap;
      margin: 0;
    }
    header .toolbar > label {
      min-width: 160px;
      flex: 0 0 160px;
    }
    .filter-bar {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
      gap: 12px;
      align-items: end;
      margin: 14px 0 18px;
      padding: 14px;
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
    }
    .filter-bar label { margin: 0; }
    .filter-actions {
      display: flex;
      align-items: end;
      gap: 8px;
      min-height: 38px;
    }
    label {
      display: grid;
      gap: 5px;
      margin: 10px 0;
      color: var(--muted);
      font-size: 12px;
    }
    .hint {
      color: var(--muted);
      font-size: 11px;
      line-height: 1.35;
    }
    .money-quota-field {
      margin: 12px 0;
      padding: 12px;
      border: 1px solid var(--line);
      border-radius: 7px;
      background: #f8fafc;
    }
    .money-quota-field > label:first-child { margin-top: 0; }
    .money-input-row {
      display: grid;
      grid-template-columns: 100px minmax(0, 1fr);
      gap: 8px;
    }
    .money-preview {
      min-height: 20px;
      margin-top: 7px;
      color: var(--brand-dark);
      font-size: 12px;
      font-weight: 600;
    }
    .unlimited-toggle {
      display: flex;
      align-items: center;
      gap: 7px;
      margin: 9px 0 0;
      color: var(--text);
    }
    .unlimited-toggle input { width: auto; margin: 0; }
    .quota-advanced { margin-top: 9px; color: var(--muted); }
    .quota-advanced summary { cursor: pointer; font-size: 11px; }
    .quota-advanced label { margin-bottom: 0; }
    .required-mark {
      color: var(--danger);
      font-weight: 700;
    }
    input, select, textarea {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 8px 10px;
      background: #fff;
      color: var(--text);
      font: inherit;
    }
    textarea {
      min-height: 76px;
      resize: vertical;
    }
    select[multiple] {
      min-height: 150px;
    }
    button.primary, button.secondary, button.danger {
      border: 0;
      border-radius: 6px;
      padding: 8px 12px;
      cursor: pointer;
      font-weight: 600;
    }
    button.primary { background: var(--brand); color: white; }
    button.primary:hover { background: var(--brand-dark); }
    button.secondary { background: #eef2f7; color: var(--text); }
    button.danger { background: #fee4e2; color: var(--danger); }
    .row-actions {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
      min-width: 124px;
    }
    .row-actions button { white-space: nowrap; }
    .table-scroll {
      width: 100%;
      overflow-x: auto;
      margin-top: 12px;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel);
    }
    table {
      width: max-content;
      min-width: 100%;
      border-collapse: collapse;
      background: var(--panel);
    }
    th, td {
      border-bottom: 1px solid var(--line);
      padding: 8px 10px;
      text-align: left;
      vertical-align: top;
      min-width: 112px;
      max-width: 360px;
      overflow-wrap: anywhere;
    }
    th {
      position: sticky;
      top: 0;
      z-index: 3;
      background: #f8fafc;
      color: var(--muted);
      font-size: 12px;
      font-weight: 700;
    }
    th:first-child, td:first-child { min-width: 72px; }
    th.actions, td.actions {
      position: sticky;
      right: 0;
      z-index: 2;
      min-width: 150px;
      background: var(--panel);
      box-shadow: -1px 0 var(--line);
    }
    th.actions { z-index: 4; background: #f8fafc; }
    .cell-compact { min-width: 72px; width: 1%; white-space: nowrap; }
    .cell-medium { min-width: 160px; }
    .cell-wide { min-width: 240px; }
    tr:last-child td { border-bottom: 0; }
    code, pre {
      background: var(--code);
      border-radius: 6px;
      padding: 2px 5px;
    }
    pre {
      overflow: auto;
      padding: 12px;
      max-height: 320px;
    }
    .status {
      color: var(--muted);
      font-size: 12px;
    }
    .timezone-note {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      margin: 8px 0 2px;
      padding: 5px 9px;
      border: 1px solid #b7ded9;
      border-radius: 6px;
      background: #eefaf8;
      color: var(--brand-dark);
      font-size: 12px;
      font-weight: 600;
    }
    .error { color: var(--danger); }
    .key-once {
      border: 1px solid #fedf89;
      background: #fffaeb;
      color: #7a4b00;
      padding: 10px;
      border-radius: 6px;
      margin-top: 12px;
      display: none;
    }
    @media (max-width: 820px) {
      header {
        position: static;
        align-items: stretch;
        flex-direction: column;
      }
      header .toolbar { flex-wrap: wrap; }
      header .toolbar { width: 100%; }
      header .toolbar > label { flex: 1 1 100%; }
      main { grid-template-columns: 1fr; }
      nav {
        display: flex;
        overflow-x: auto;
        border-right: 0;
        border-bottom: 1px solid var(--line);
      }
      nav button {
        width: auto;
        white-space: nowrap;
      }
      .section-heading { align-items: stretch; flex-direction: column; }
      .org-filter-bar { grid-template-columns: 1fr; }
      .org-effective-summary { grid-template-columns: 1fr; }
    }
    @media (max-width: 1180px) {
      .management-grid { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>Enterprise Policy Hub</h1>
      <div class="status" id="me">正在校验管理员身份...</div>
    </div>
    <div class="toolbar">
      <label style="min-width: 118px">
        <span>语言</span>
        <select id="language-select" aria-label="Language">
          <option value="zh">中文</option>
          <option value="en">English</option>
        </select>
      </label>
      <button class="secondary" onclick="refreshAll()">刷新</button>
      <button class="primary" onclick="syncUsage()">同步用量</button>
    </div>
  </header>
  <main>
    <nav>
      <button class="active" data-tab="overview">概览</button>
      <button data-tab="orgs">组织</button>
      <button data-tab="policies">Policy</button>
      <button data-tab="keys">企业令牌管理</button>
      <button data-tab="budgets">预算</button>
      <button data-tab="usage">用量</button>
      <button data-tab="tokenop">TokenOperation</button>
      <button data-tab="admins">Hub 权限</button>
      <button data-tab="audit">审计</button>
    </nav>

    <div>
      <section id="overview" class="active">
        <h2>运行状态</h2>
        <div class="grid">
          <div class="panel"><h3>当前身份</h3><pre id="me-json">{}</pre></div>
          <div class="panel"><h3>最近同步结果</h3><pre id="sync-result">尚未同步</pre></div>
        </div>
      </section>

      <section id="orgs">
        <div class="section-heading">
          <div>
            <h2>组织架构</h2>
            <p>按层级查看组织节点，以及 Policy、group 和所属账号的实际来源。</p>
          </div>
          <button class="primary" type="button" onclick="beginOrgCreate(0)">创建顶层组织</button>
        </div>
        <div class="panel org-explorer">
          <div class="org-filter-bar">
            <label>搜索组织
              <input id="org-search" type="search" data-placeholder="输入名称、编码或 ID" placeholder="输入名称、编码或 ID">
            </label>
            <label>组织类型
              <select id="org-type-filter">
                <option value="">全部类型</option>
                <option value="company">公司</option>
                <option value="business_unit">事业部</option>
                <option value="department">部门</option>
                <option value="team">团队</option>
                <option value="project">项目</option>
                <option value="cost_center">成本中心</option>
              </select>
            </label>
            <label>状态
              <select id="org-status-filter">
                <option value="">全部状态</option>
                <option value="enabled">启用</option>
                <option value="disabled">禁用</option>
              </select>
            </label>
            <div class="filter-actions">
              <button class="secondary" type="button" onclick="expandAllOrgs()">全部展开</button>
              <button class="secondary" type="button" onclick="collapseAllOrgs()">全部收起</button>
            </div>
          </div>
          <div id="org-tree"></div>
        </div>

        <div class="org-drawer-backdrop" id="org-drawer" aria-hidden="true" onclick="handleOrgDrawerBackdrop(event)">
          <aside class="org-drawer" role="dialog" aria-modal="true" aria-labelledby="org-form-title">
            <div class="org-drawer-header">
              <div>
                <h3 id="org-form-title">创建组织节点</h3>
                <div class="org-breadcrumb" id="org-breadcrumb">顶层组织</div>
              </div>
              <button class="org-drawer-close" type="button" onclick="closeOrgDrawer()" title="关闭" aria-label="关闭">&times;</button>
            </div>
            <div class="org-drawer-body">
              <div class="org-effective-summary" id="org-effective-summary" hidden></div>
              <form id="org-form">
                <label><span>名称 <span class="required-mark" aria-hidden="true">*</span></span><input name="name" required></label>
                <label>编码 <input name="code"></label>
                <label>父级组织
                  <select name="parent_id" data-ref-select="org_units" data-placeholder="顶层组织"></select>
                  <span class="hint">创建下级时自动选择父组织；现有节点暂不支持移动。</span>
                </label>
                <label>类型
                  <select name="type">
                    <option value="department">部门</option>
                    <option value="company">公司</option>
                    <option value="business_unit">事业部</option>
                    <option value="team">团队</option>
                    <option value="project">项目</option>
                    <option value="cost_center">成本中心</option>
                  </select>
                </label>
                <label>默认 Policy
                  <select name="default_policy_id" data-ref-select="policies" data-placeholder="不绑定，按上级继承"></select>
                </label>
                <label>默认 group
                  <select name="default_group" data-ref-select="groups" data-placeholder="不覆盖"></select>
                </label>
                <label>所属账号
                  <select name="newapi_user_id" data-ref-select="users" data-placeholder="稍后指定"></select>
                  <span class="hint">企业令牌继承当前组织节点的所属账号，不会继续向上级组织查找。</span>
                </label>
                <label>状态
                  <select name="status">
                    <option value="enabled">启用</option>
                    <option value="disabled">禁用</option>
                  </select>
                </label>
                <div class="toolbar org-form-actions">
                  <button class="primary" id="org-submit" type="submit">创建</button>
                  <button class="secondary" type="button" onclick="closeOrgDrawer()">取消</button>
                </div>
              </form>
            </div>
          </aside>
        </div>
      </section>

      <section id="policies">
        <h2>Policy 管理</h2>
        <div class="grid management-grid">
          <form class="panel" id="policy-form">
            <h3 id="policy-form-title">创建 Policy</h3>
            <label><span>名称 <span class="required-mark" aria-hidden="true">*</span></span><input name="name" required></label>
            <label>默认 group
              <select name="default_group" data-ref-select="groups" data-placeholder="不设置"></select>
            </label>
            <label>允许模型
              <select name="allowed_models" multiple size="8" data-ref-select="models"></select>
              <span class="hint">从 new-api enabled abilities 和渠道模型列表读取；按住 Ctrl / Cmd 可多选。</span>
            </label>
            <label>禁止模型
              <select name="denied_models" multiple size="8" data-ref-select="models"></select>
            </label>
            <div class="money-quota-field" data-money-quota="monthly_budget" data-unlimited="true">
              <label>月预算金额
                <div class="money-input-row"><select name="monthly_budget_currency" data-money-currency></select><input name="monthly_budget_amount" type="number" min="0" step="0.01" inputmode="decimal"></div>
                <span class="hint">按预算时区的自然月限制该 Policy 所绑定组织或令牌的合计用量。</span>
              </label>
              <label class="unlimited-toggle"><input name="monthly_budget_unlimited" type="checkbox" checked> 不限月预算</label>
              <div class="money-preview" data-money-preview></div>
              <details class="quota-advanced"><summary>高级：内部计费额度</summary><label>月预算 quota <input name="monthly_budget_quota" type="number" min="0" data-raw-quota></label></details>
            </div>
            <div class="money-quota-field" data-money-quota="daily_budget" data-unlimited="true">
              <label>日预算金额
                <div class="money-input-row"><select name="daily_budget_currency" data-money-currency></select><input name="daily_budget_amount" type="number" min="0" step="0.01" inputmode="decimal"></div>
                <span class="hint">按预算时区的自然日自动创建周期。</span>
              </label>
              <label class="unlimited-toggle"><input name="daily_budget_unlimited" type="checkbox" checked> 不限日预算</label>
              <div class="money-preview" data-money-preview></div>
              <details class="quota-advanced"><summary>高级：内部计费额度</summary><label>日预算 quota <input name="daily_budget_quota" type="number" min="0" data-raw-quota></label></details>
            </div>
            <div class="money-quota-field" data-money-quota="key_default" data-unlimited="true">
              <label>令牌默认金额
                <div class="money-input-row"><select name="key_default_currency" data-money-currency></select><input name="key_default_amount" type="number" min="0" step="0.01" inputmode="decimal"></div>
                <span class="hint">单个令牌的总额度上限；重复同步不会重新充值。</span>
              </label>
              <label class="unlimited-toggle"><input name="key_default_unlimited" type="checkbox" checked> 令牌额度不限</label>
              <div class="money-preview" data-money-preview></div>
              <details class="quota-advanced"><summary>高级：内部计费额度</summary><label>令牌默认 quota <input name="key_default_quota" type="number" min="0" data-raw-quota></label></details>
            </div>
            <label>状态
              <select name="status">
                <option value="enabled">启用</option>
                <option value="disabled">禁用</option>
              </select>
            </label>
            <div class="toolbar">
              <button class="primary" id="policy-submit" type="submit">创建</button>
              <button class="secondary" id="policy-cancel-edit" type="button" onclick="resetPolicyForm()" hidden>取消编辑</button>
            </div>
          </form>
          <div class="panel">
            <h3>Policy 列表</h3>
            <div id="policy-table"></div>
          </div>
        </div>
      </section>

      <section id="keys">
        <h2>企业令牌管理</h2>
        <div class="grid management-grid">
          <form class="panel" id="key-form">
            <h3 id="key-form-title">创建企业令牌</h3>
            <label><span>名称 <span class="required-mark" aria-hidden="true">*</span></span><input name="name" required></label>
            <label>组织
              <select name="org_unit_id" data-ref-select="org_units" data-placeholder="不绑定组织"></select>
            </label>
            <label>项目
              <select name="project_id" data-ref-select="projects" data-placeholder="不绑定项目"></select>
            </label>
            <label>成本中心
              <select name="cost_center_id" data-ref-select="cost_centers" data-placeholder="不绑定成本中心"></select>
            </label>
            <label>Policy
              <select name="policy_id" data-ref-select="policies" data-placeholder="按组织继承"></select>
            </label>
            <label>所属账号
              <select name="newapi_user_id" data-ref-select="users" data-placeholder="按组织继承"></select>
            </label>
            <label>环境
              <select name="environment">
                <option value="prod">prod</option>
                <option value="staging">staging</option>
                <option value="dev">dev</option>
                <option value="test">test</option>
              </select>
            </label>
            <label>用途 <textarea name="purpose"></textarea></label>
            <label>联系人 <input name="contact"></label>
            <label>状态
              <select name="status">
                <option value="enabled">启用</option>
                <option value="disabled">禁用</option>
              </select>
            </label>
            <div class="toolbar">
              <button class="primary" id="key-submit" type="submit">创建并同步</button>
              <button class="secondary" id="key-cancel-edit" type="button" onclick="resetKeyForm()" hidden>取消编辑</button>
            </div>
            <div class="key-once" id="new-key"></div>
          </form>
          <div class="panel">
            <h3>企业令牌列表</h3>
            <div id="key-table"></div>
          </div>
        </div>
      </section>

      <section id="budgets">
        <h2>预算</h2>
        <p class="status">Policy 日/月预算按 <strong id="budget-timezone">预算时区</strong> 的自然周期执行；手工预算使用下方选择的精确时间范围。</p>
        <div class="grid management-grid">
          <form class="panel" id="budget-form">
            <h3 id="budget-form-title">创建手工预算</h3>
            <label>范围类型
              <select name="scope_type">
                <option value="org_unit">org_unit</option>
                <option value="enterprise_key">enterprise_key</option>
                <option value="project">project</option>
                <option value="cost_center">cost_center</option>
              </select>
            </label>
            <label><span>范围对象 <span class="required-mark" aria-hidden="true">*</span></span>
              <select name="scope_id" id="budget-scope-id" required></select>
              <span class="hint">会根据上面的范围类型自动切换组织、企业令牌、项目或成本中心。</span>
            </label>
            <label>开始时间
              <input name="period_start" type="datetime-local" data-unix-seconds="true">
              <span class="hint">留空表示立即生效，不限制开始时间。</span>
            </label>
            <label>结束时间
              <input name="period_end" type="datetime-local" data-unix-seconds="true">
              <span class="hint">留空表示长期有效，不限制结束时间。</span>
            </label>
            <div class="money-quota-field" data-money-quota="budget">
              <label><span>预算金额 <span class="required-mark" aria-hidden="true">*</span></span>
                <div class="money-input-row"><select name="budget_currency" data-money-currency></select><input name="budget_amount" type="number" min="0.01" step="0.01" inputmode="decimal" required></div>
              </label>
              <div class="money-preview" data-money-preview></div>
              <details class="quota-advanced"><summary>高级：内部计费额度</summary><label>预算 quota <input name="budget_quota" type="number" min="1" data-raw-quota></label></details>
            </div>
            <label>状态
              <select name="status">
                <option value="enabled">启用</option>
                <option value="disabled">禁用</option>
              </select>
            </label>
            <div class="toolbar">
              <button class="primary" id="budget-submit" type="submit">创建</button>
              <button class="secondary" id="budget-cancel-edit" type="button" onclick="resetBudgetForm()" hidden>取消编辑</button>
            </div>
          </form>
          <div class="panel">
            <h3>预算列表</h3>
            <div id="budget-table"></div>
          </div>
        </div>
      </section>

      <section id="usage">
        <h2>用量报表</h2>
        <div class="timezone-note"><span>报表与预算时区</span><strong id="usage-timezone">UTC</strong></div>
        <div class="filter-bar">
          <label>汇总维度
            <select id="usage-group">
              <option value="org_unit">组织</option>
              <option value="key">企业令牌</option>
              <option value="model">模型</option>
              <option value="channel">渠道</option>
              <option value="project">项目</option>
              <option value="cost_center">成本中心</option>
            </select>
          </label>
          <label>组织
            <select id="usage-org" data-ref-select="org_units" data-placeholder="全部组织"></select>
          </label>
          <label>企业令牌
            <select id="usage-key" data-ref-select="enterprise_keys" data-placeholder="全部企业令牌"></select>
          </label>
          <label>模型
            <select id="usage-model" data-ref-select="models" data-placeholder="全部模型"></select>
          </label>
          <label>渠道
            <select id="usage-channel" data-ref-select="channels" data-placeholder="全部渠道"></select>
          </label>
          <label>项目
            <select id="usage-project" data-ref-select="projects" data-placeholder="全部项目"></select>
          </label>
          <label>成本中心
            <select id="usage-cost-center" data-ref-select="cost_centers" data-placeholder="全部成本中心"></select>
          </label>
          <label>开始时间
            <input id="usage-start" type="datetime-local">
          </label>
          <label>结束时间（不含）
            <input id="usage-end" type="datetime-local">
          </label>
          <div class="filter-actions">
            <button class="primary" type="button" onclick="loadUsage()">应用筛选</button>
            <button class="secondary" type="button" onclick="resetUsageFilters()">清空筛选</button>
          </div>
        </div>
        <div class="grid report-grid">
          <div class="panel"><h3>汇总</h3><div id="usage-summary"></div></div>
          <div class="panel"><h3>明细</h3><div id="usage-details"></div></div>
        </div>
      </section>

      <section id="admins">
        <h2>Hub 权限</h2>
        <div class="grid management-grid">
          <form class="panel" id="admin-form">
            <h3>授权管理员</h3>
            <label><span>用户列表 <span class="required-mark" aria-hidden="true">*</span></span>
              <select name="newapi_user_id" id="admin-user-select" data-ref-select="users" data-placeholder="选择用户" required></select>
            </label>
            <label>用户名 <input name="newapi_username" id="admin-username-input" readonly></label>
            <label>Hub 角色
              <select name="hub_role">
                <option value="hub_org_admin">hub_org_admin</option>
                <option value="hub_key_admin">hub_key_admin</option>
                <option value="hub_finance_admin">hub_finance_admin</option>
                <option value="hub_auditor">hub_auditor</option>
                <option value="hub_super_admin">hub_super_admin</option>
              </select>
            </label>
            <label>组织范围
              <select name="scope_org_unit_id" data-ref-select="org_units" data-placeholder="全局"></select>
            </label>
            <button class="primary" type="submit">授权</button>
          </form>
          <div class="panel">
            <h3>授权列表</h3>
            <div id="admin-table"></div>
          </div>
        </div>
      </section>

      <section id="tokenop">
        <h2>TokenOperation 对接</h2>
        <div class="grid">
          <div class="panel">
            <h3>配置状态</h3>
            <pre id="tokenop-status">{}</pre>
            <div class="toolbar">
              <button class="secondary" onclick="loadTokenOperation()">刷新状态</button>
              <button class="primary" onclick="syncTokenOperationObjects()">同步对象清单</button>
            </div>
          </div>
          <div class="panel">
            <h3>最近同步结果</h3>
            <pre id="tokenop-result">尚未同步</pre>
          </div>
          <div class="panel">
            <h3>客户侧结算明细</h3>
            <label>limit <input id="tokenop-usage-limit" type="number" value="50"></label>
            <div class="toolbar">
              <button class="secondary" onclick="loadTokenOperationUsageDetails()">读取明细</button>
            </div>
            <pre id="tokenop-usage-details">尚未读取</pre>
          </div>
        </div>
      </section>

      <section id="audit">
        <h2>审计与同步</h2>
        <div class="grid">
          <div class="panel"><h3>审计日志</h3><div id="audit-table"></div></div>
          <div class="panel"><h3>同步任务</h3><div id="sync-table"></div></div>
        </div>
      </section>
    </div>
  </main>

  <script>
    const pageBase = window.location.pathname.endsWith('/') ? window.location.pathname : window.location.pathname + '/';
    const apiBase = pageBase + 'api/';
    const state = {
      language: window.localStorage.getItem('eph-language') === 'en' ? 'en' : 'zh',
      orgExpanded: {},
      orgTreeInitialized: false,
      selectedOrgId: 0
    };
    const en = {
      '语言': 'Language', '刷新': 'Refresh', '同步用量': 'Sync usage', '概览': 'Overview',
      '组织': 'Organization', 'Policy': 'Policy', '企业令牌管理': 'Enterprise Tokens', '预算': 'Budgets',
      '用量': 'Usage', 'Hub 权限': 'Hub Access', '审计': 'Audit', '运行状态': 'Runtime status',
      '当前身份': 'Current identity', '最近同步结果': 'Latest sync result', '尚未同步': 'Not synced yet',
      '正在校验管理员身份...': 'Validating administrator identity...',
      '组织架构': 'Organization structure', '创建组织节点': 'Create organization node', '名称': 'Name',
      '编码': 'Code', '父级组织': 'Parent organization', '类型': 'Type', '默认 Policy': 'Default Policy',
      '所属账号': 'Account owner', '状态': 'Status',
      '创建': 'Create', '取消编辑': 'Cancel editing', '组织列表': 'Organizations',
      '从 Hub 已有组织读取；留空表示创建顶层组织。': 'Loaded from Hub organizations; leave blank to create a root organization.',
      'Policy 管理': 'Policy management', '创建 Policy': 'Create Policy', '默认 group': 'Default group',
      '允许模型': 'Allowed models', '禁止模型': 'Denied models', '月预算 quota': 'Monthly budget quota',
      '日预算 quota': 'Daily budget quota', '令牌默认 quota': 'Default token quota',
      '从 new-api enabled abilities 和渠道模型列表读取；按住 Ctrl / Cmd 可多选。': 'Loaded from enabled new-api abilities and channel models; hold Ctrl / Cmd to select multiple.',
      '按自然月限制该 Policy 所绑定组织或 Key 的合计用量，0 表示不限。': 'Limits total usage for organizations or tokens attached to this Policy by calendar month; 0 means unlimited.',
      '按预算时区的自然日自动创建周期，0 表示不限。': 'Creates calendar-day periods in the budget timezone; 0 means unlimited.',
      '单个令牌的总额度上限；重复同步不会重新充值。': 'Total quota cap for one token; repeated synchronization does not refill it.',
      'Policy 列表': 'Policies', '创建企业令牌': 'Create enterprise token', '企业令牌列表': 'Enterprise tokens',
      '项目': 'Project', '成本中心': 'Cost center', '环境': 'Environment', '用途': 'Purpose', '联系人': 'Contact',
      '创建并同步': 'Create and sync', '预算时区': 'budget timezone', '创建手工预算': 'Create manual budget',
      '范围类型': 'Scope type', '范围对象': 'Scope object', '开始时间': 'Start time', '结束时间': 'End time',
      '结束时间（不含）': 'End time (exclusive)', '报表与预算时区': 'Reporting and budget timezone',
      '预算 quota': 'Budget quota', '预算列表': 'Budgets', '手工预算': 'manual budget',
      '会根据上面的范围类型自动切换组织、企业令牌、项目或成本中心。': 'Options change automatically based on the selected scope type.',
      '留空表示立即生效，不限制开始时间。': 'Leave blank for immediate effect with no start limit.',
      '留空表示长期有效，不限制结束时间。': 'Leave blank for no end limit.',
      '用量报表': 'Usage report', '汇总维度': 'Group by', '企业令牌': 'Enterprise token', '模型': 'Model',
      '渠道': 'Channel', '全部组织': 'All organizations', '全部企业令牌': 'All enterprise tokens',
      '全部模型': 'All models', '全部渠道': 'All channels', '全部项目': 'All projects',
      '全部成本中心': 'All cost centers', '应用筛选': 'Apply filters', '清空筛选': 'Clear filters',
      '汇总': 'Summary', '明细': 'Details', '授权管理员': 'Authorize administrator',
      '用户列表': 'User list', '用户名': 'Username', 'Hub 角色': 'Hub role', '组织范围': 'Organization scope',
      '授权': 'Authorize', '授权列表': 'Authorizations', 'TokenOperation 对接': 'TokenOperation integration',
      '配置状态': 'Configuration status', '刷新状态': 'Refresh status', '同步对象清单': 'Sync object inventory',
      '客户侧结算明细': 'Customer settlement details', '读取明细': 'Load details', '尚未读取': 'Not loaded yet',
      '审计与同步': 'Audit and synchronization', '审计日志': 'Audit logs', '同步任务': 'Sync jobs',
      'Policy 日/月预算按': 'Policy daily/monthly budgets follow the',
      '的自然周期执行；手工预算使用下方选择的精确时间范围。': 'calendar periods; manual budgets use the exact range selected below.',
      '启用': 'Enabled', '禁用': 'Disabled', '请选择': 'Select', '全局': 'Global', '不设置': 'Not set',
      '不覆盖': 'No override', '不绑定，按上级继承': 'Not bound; inherit from parent', '继承或稍后指定': 'Inherit or specify later',
      '不绑定组织': 'No organization', '不绑定项目': 'No project', '不绑定成本中心': 'No cost center',
      '按组织继承': 'Inherit from organization', '选择用户': 'Select user',
      '顶层组织': 'Root organization', '请选择范围对象': 'Select a scope object', '暂无数据': 'No data',
      '操作': 'Actions', '编辑': 'Edit', '删除': 'Delete', '同步': 'Sync', '轮换': 'Rotate',
      '保存修改': 'Save changes', '保存并同步': 'Save and sync', '归零': 'Reset', '通过 Policy 管理': 'Managed by Policy',
      'ID': 'ID', '路径': 'Path', '允许模型列表': 'Allowed models', '禁止模型列表': 'Denied models',
      '月预算': 'Monthly budget', '日预算': 'Daily budget', '直接 Policy': 'Direct Policy',
      '有效 Policy': 'Effective Policies', '指纹': 'Fingerprint', '管理员状态': 'Admin status',
      '生效状态': 'Effective status', '预算阻断': 'Budget blocks', '范围': 'Scope', '来源': 'Source',
      '周期': 'Period', '已结算': 'Confirmed', '待结算': 'Pending', '合计': 'Total',
      '阻断令牌': 'Blocked tokens', '维度': 'Dimension', '已结算 quota': 'Confirmed quota',
      '待结算 quota': 'Pending quota', '合计 quota': 'Total quota', '已结算金额': 'Confirmed amount',
      '待结算金额': 'Pending amount', '合计金额': 'Total amount', '记录数': 'Records',
      '任务 ID': 'Task ID', '结算状态': 'Settlement status', '管理员': 'Administrator', '动作': 'Action',
      '对象': 'Target', '对象 ID': 'Target', '时间': 'Time', '实体': 'Entity', '实体 ID': 'Entity',
      '错误': 'Error', '完整 Key 只展示一次：': 'The full token is shown only once:', '不限': 'Unlimited',
      '日期时间格式无效': 'Invalid date/time format', '请求失败': 'Request failed',
      'department': 'Department', 'company': 'Company', 'business_unit': 'Business unit', 'team': 'Team',
      'project': 'Project', 'cost_center': 'Cost center', 'enabled': 'Enabled', 'disabled': 'Disabled',
      'pending': 'Pending', 'confirmed': 'Confirmed', 'failed': 'Failed', 'success': 'Success',
      'manual': 'Manual', 'policy': 'Policy', 'daily': 'Daily', 'monthly': 'Monthly', 'custom': 'Custom',
      'org_unit': 'Organization', 'enterprise_key': 'Enterprise token', 'key': 'Enterprise token',
      'model': 'Model', 'channel': 'Channel',
      '按层级查看组织节点，以及 Policy、group 和所属账号的实际来源。': 'Browse the organization hierarchy and the actual sources of policies, groups, and account owners.',
      '创建顶层组织': 'Create root organization', '搜索组织': 'Search organizations', '输入名称、编码或 ID': 'Search by name, code, or ID',
      '组织类型': 'Organization type', '全部类型': 'All types', '全部状态': 'All statuses', '全部展开': 'Expand all', '全部收起': 'Collapse all',
      '展开': 'Expand', '收起': 'Collapse',
      '公司': 'Company', '事业部': 'Business unit', '部门': 'Department', '团队': 'Team',
      '组织节点': 'Organization', '生效 Policy 链': 'Effective policy chain', '生效 group': 'Effective group',
      '直接配置': 'Direct', '继承自 {name}': 'Inherited from {name}', '由 Policy {name} 设置': 'Set by policy {name}',
      '未设置': 'Not set', '没有符合条件的组织节点': 'No organizations match the filters', '添加下级': 'Add child',
      '关闭': 'Close', '取消': 'Cancel', '创建下级组织': 'Create child organization',
      '创建下级时自动选择父组织；现有节点暂不支持移动。': 'The parent is selected automatically when creating a child. Existing nodes cannot be moved yet.',
      '稍后指定': 'Specify later', '企业令牌继承当前组织节点的所属账号，不会继续向上级组织查找。': 'Enterprise tokens inherit the account owner from this organization only; parent organizations are not searched.',
      '月预算': 'Monthly budget', '日预算': 'Daily budget', 'Policy 交集': 'Policy intersection', '本级备用值': 'Direct fallback',
      '月预算金额': 'Monthly budget amount', '日预算金额': 'Daily budget amount', '令牌默认金额': 'Default token amount',
      '预算金额': 'Budget amount', '不限月预算': 'Unlimited monthly budget', '不限日预算': 'Unlimited daily budget',
      '令牌额度不限': 'Unlimited token quota', '高级：内部计费额度': 'Advanced: internal billing quota',
      '令牌额度': 'Token quota',
      '内部计费额度': 'Internal billing quota', '按预算时区的自然月限制该 Policy 所绑定组织或令牌的合计用量。': 'Limits total usage for organizations or tokens attached to this Policy by calendar month in the budget timezone.',
      '按预算时区的自然日自动创建周期。': 'Creates calendar-day periods in the budget timezone.',
      '金额按提交时汇率换算，保存后不会随汇率变化。': 'Converted using the exchange rate at submission time; saved budgets do not change with later rates.',
      '请输入金额': 'Enter an amount'
    };

    function t(key, values) {
      let text = state.language === 'en' && en[key] ? en[key] : key;
      for (const name of Object.keys(values || {})) {
        text = text.replaceAll('{' + name + '}', String(values[name]));
      }
      return text;
    }

    function applyLanguage() {
      document.documentElement.lang = state.language === 'en' ? 'en' : 'zh-CN';
      document.getElementById('language-select').value = state.language;
      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      let node;
      while ((node = walker.nextNode())) {
        if (node.parentElement && ['SCRIPT', 'STYLE', 'PRE', 'CODE'].includes(node.parentElement.tagName)) continue;
        const original = node.__ephI18nKey === undefined ? node.nodeValue.trim() : node.__ephI18nKey;
        if (!original) continue;
        node.__ephI18nKey = original;
        const translated = t(original);
        node.nodeValue = node.nodeValue.replace(node.nodeValue.trim(), translated);
      }
      document.querySelectorAll('[data-placeholder]').forEach(element => {
        if (!element.dataset.i18nPlaceholder) element.dataset.i18nPlaceholder = element.dataset.placeholder;
        element.dataset.placeholder = t(element.dataset.i18nPlaceholder);
        if ('placeholder' in element) element.placeholder = element.dataset.placeholder;
      });
    }

    function getNewAPIUserId() {
      try {
        const uid = window.localStorage.getItem('uid');
        if (uid) return uid;
        const rawUser = window.localStorage.getItem('user');
        if (rawUser) {
          const user = JSON.parse(rawUser);
          if (user && user.id) return String(user.id);
        }
      } catch (error) {
        return '';
      }
      return '';
    }

    function api(path, options) {
      const requestOptions = Object.assign({}, options || {});
      const headers = Object.assign({ 'Content-Type': 'application/json' }, requestOptions.headers || {});
      const uid = getNewAPIUserId();
      if (uid) headers['New-Api-User'] = uid;
      requestOptions.headers = headers;
      return fetch(apiBase + path.replace(/^\//, ''), Object.assign({
        credentials: 'include'
      }, requestOptions)).then(async response => {
        const text = await response.text();
        let payload = {};
        try { payload = text ? JSON.parse(text) : {}; } catch (error) { payload = { success: false, message: text }; }
        if (!response.ok || payload.success === false) {
          throw new Error(payload.message || response.statusText);
        }
        return payload.data;
      });
    }

    function formJSON(form) {
      const data = {};
      for (const input of Array.from(form.elements)) {
        if (!input.name || input.disabled || input.type === 'submit' || input.type === 'button') continue;
        if (input.type === 'checkbox') {
          data[input.name] = input.checked;
          continue;
        }
        if (input.tagName === 'SELECT' && input.multiple) {
          data[input.name] = Array.from(input.selectedOptions).map(option => option.value).filter(Boolean);
          continue;
        }
        const trimmed = String(input.value || '').trim();
        if (input.dataset.unixSeconds === 'true') {
          const seconds = trimmed === '' ? 0 : zonedLocalToUnix(trimmed);
          if (trimmed !== '' && !Number.isFinite(seconds)) {
            throw new Error(t('日期时间格式无效'));
          }
          data[input.name] = seconds;
        } else if (input.type === 'number' || input.tagName === 'SELECT' && input.dataset.numeric === 'true') {
          data[input.name] = trimmed === '' ? 0 : Number(trimmed);
        } else if (input.name === 'allowed_models' || input.name === 'denied_models') {
          data[input.name] = trimmed ? trimmed.split(',').map(x => x.trim()).filter(Boolean) : [];
        } else {
          data[input.name] = trimmed;
        }
      }
      return data;
    }

    function quotaCurrencyConfig() {
      return (state.reference && state.reference.quota_currency) || {
        quota_per_unit: 500000, display_type: 'USD', currency_symbol: '$', usd_exchange_rate: 7.3,
        custom_currency_symbol: '¤', custom_currency_exchange_rate: 1
      };
    }

    function normalizedDisplayCurrency() {
      const display = String(quotaCurrencyConfig().display_type || 'USD').toUpperCase();
      return ['USD', 'CNY', 'CUSTOM'].includes(display) ? display : 'USD';
    }

    function currencyRate(currency) {
      const config = quotaCurrencyConfig();
      if (currency === 'CNY') return Number(config.usd_exchange_rate || 0);
      if (currency === 'CUSTOM') return Number(config.custom_currency_exchange_rate || 0);
      if (currency === 'QUOTA') return Number(config.quota_per_unit || 500000);
      return 1;
    }

    function currencySymbol(currency) {
      const config = quotaCurrencyConfig();
      if (currency === 'USD') return '$';
      if (currency === 'CNY') return '¥';
      if (currency === 'CUSTOM') return config.custom_currency_symbol || '¤';
      return '';
    }

    function quotaFromAmount(amount, currency) {
      const value = Number(amount);
      const rate = currencyRate(currency);
      if (!Number.isFinite(value) || !Number.isFinite(rate) || rate <= 0) return 0;
      return Math.round(value * Number(quotaCurrencyConfig().quota_per_unit || 500000) / rate);
    }

    function amountFromQuota(quota, currency, quotaPerUnit, exchangeRate) {
      const unit = Number(quotaPerUnit || quotaCurrencyConfig().quota_per_unit || 500000);
      const rate = Number(exchangeRate || currencyRate(currency));
      if (!unit || !Number.isFinite(rate)) return 0;
      return Number(quota || 0) * rate / unit;
    }

    function formatCurrencyAmount(amount, currency) {
      const value = Number(amount || 0);
      const formatted = new Intl.NumberFormat(state.language === 'en' ? 'en-US' : 'zh-CN', {
        minimumFractionDigits: 2, maximumFractionDigits: 6
      }).format(Number.isFinite(value) ? value : 0);
      return currency === 'QUOTA' ? formatted + ' quota' : currencySymbol(currency) + formatted + ' ' + currency;
    }

    function formatUSDInDisplayCurrency(amount) {
      const currency = normalizedDisplayCurrency();
      return formatCurrencyAmount(Number(amount || 0) * currencyRate(currency), currency);
    }

    function formatQuotaWithMoney(quota, amount, currency, quotaPerUnit, exchangeRate) {
      const quotaValue = Number(quota || 0);
      if (quotaValue <= 0) return t('不限');
      const displayCurrency = currency || normalizedDisplayCurrency();
      const displayAmount = amount !== null && amount !== undefined && String(amount) !== ''
        ? Number(amount)
        : amountFromQuota(quotaValue, displayCurrency, quotaPerUnit, exchangeRate);
      return formatCurrencyAmount(displayAmount, displayCurrency) + ' · ' + quotaValue.toLocaleString() + ' quota';
    }

    function populateMoneyCurrencySelect(select) {
      const selected = select.value || normalizedDisplayCurrency();
      const customSymbol = quotaCurrencyConfig().custom_currency_symbol || '¤';
      select.innerHTML = [
        { value: 'USD', label: 'USD ($)' }, { value: 'CNY', label: 'CNY (¥)' },
        { value: 'CUSTOM', label: 'CUSTOM (' + customSymbol + ')' }
      ].map(item => '<option value="' + item.value + '">' + escapeHTML(item.label) + '</option>').join('');
      select.value = ['USD', 'CNY', 'CUSTOM'].includes(selected) ? selected : normalizedDisplayCurrency();
    }

    function syncMoneyQuotaField(container, source) {
      const prefix = container.dataset.moneyQuota;
      const form = container.closest('form');
      const amount = form.elements[prefix + '_amount'];
      const currency = form.elements[prefix + '_currency'];
      const rawQuota = form.elements[prefix + '_quota'];
      const unlimited = form.elements[prefix + '_unlimited'];
      const isUnlimited = Boolean(unlimited && unlimited.checked);
      if (amount) amount.disabled = isUnlimited;
      if (rawQuota) rawQuota.disabled = isUnlimited;
      if (isUnlimited) {
        if (rawQuota) rawQuota.value = '0';
        container.querySelector('[data-money-preview]').textContent = t('不限');
        return;
      }
      if (source === 'quota' && rawQuota && amount) {
        amount.value = rawQuota.value === '' ? '' : String(Number(amountFromQuota(rawQuota.value, currency.value).toFixed(6)));
      } else if (amount && rawQuota) {
        rawQuota.value = amount.value === '' ? '' : String(quotaFromAmount(amount.value, currency.value));
      }
      const quota = Number(rawQuota && rawQuota.value || 0);
      const preview = quota > 0
        ? '≈ ' + quota.toLocaleString() + ' quota · ' + t('金额按提交时汇率换算，保存后不会随汇率变化。')
        : t('请输入金额');
      container.querySelector('[data-money-preview]').textContent = preview;
    }

    function initializeMoneyQuotaFields() {
      document.querySelectorAll('[data-money-quota]').forEach(container => {
        const select = container.querySelector('[data-money-currency]');
        populateMoneyCurrencySelect(select);
        if (container.dataset.moneyReady !== 'true') {
          const amount = container.querySelector('input[name$="_amount"]');
          const rawQuota = container.querySelector('[data-raw-quota]');
          const unlimited = container.querySelector('input[name$="_unlimited"]');
          amount.addEventListener('input', () => syncMoneyQuotaField(container, 'amount'));
          select.addEventListener('change', () => syncMoneyQuotaField(container, 'amount'));
          rawQuota.addEventListener('input', () => syncMoneyQuotaField(container, 'quota'));
          if (unlimited) unlimited.addEventListener('change', () => syncMoneyQuotaField(container, 'unlimited'));
          container.dataset.moneyReady = 'true';
        }
        syncMoneyQuotaField(container, 'amount');
      });
    }

    function resetMoneyQuotaFields(form) {
      form.querySelectorAll('[data-money-currency]').forEach(select => { select.value = normalizedDisplayCurrency(); });
      initializeMoneyQuotaFields();
    }

    function setMoneyQuotaField(form, prefix, row) {
      const quota = Number(row[prefix + '_quota'] || 0);
      const storedCurrency = row[prefix + '_currency'] || normalizedDisplayCurrency();
      const storedAmount = row[prefix + '_amount'];
      setFormValue(form, prefix + '_currency', storedCurrency);
      setFormValue(form, prefix + '_quota', quota);
      setFormValue(form, prefix + '_unlimited', quota <= 0);
      const amount = storedAmount !== null && storedAmount !== undefined && String(storedAmount) !== ''
        ? storedAmount
        : amountFromQuota(quota, storedCurrency, row[prefix + '_quota_per_unit'], row[prefix + '_exchange_rate']);
      setFormValue(form, prefix + '_amount', quota > 0 ? Number(Number(amount).toFixed(6)) : '');
      const container = form.querySelector('[data-money-quota="' + prefix + '"]');
      if (container) syncMoneyQuotaField(container, 'amount');
    }

    async function loadReference() {
      state.reference = await api('reference');
      populateReferenceControls();
    }

    function organizationTreeRows(rows) {
      const ordered = Array.isArray(rows) ? rows : [];
      const byID = new Map(ordered.map(row => [Number(row.id), row]));
      const children = new Map();
      const roots = [];
      for (const row of ordered) {
        const parentID = Number(row.parent_id || 0);
        if (!parentID || !byID.has(parentID) || parentID === Number(row.id)) {
          roots.push(row);
          continue;
        }
        if (!children.has(parentID)) children.set(parentID, []);
        children.get(parentID).push(row);
      }
      const result = [];
      const visited = new Set();
      function walk(row, depth) {
        const id = Number(row.id);
        if (visited.has(id)) return;
        visited.add(id);
        result.push({ row, depth, children: children.get(id) || [] });
        for (const child of children.get(id) || []) walk(child, depth + 1);
      }
      for (const root of roots) walk(root, 0);
      for (const row of ordered) {
        if (!visited.has(Number(row.id))) walk(row, 0);
      }
      return result;
    }

    function organizationOptions(rows) {
      return organizationTreeRows(rows).map(item => ({
        value: item.row.id,
        label: (item.depth ? Array(item.depth + 1).join('  ') + '↳ ' : '') + item.row.name +
          ' (ID: ' + item.row.id + ') / ' + orgTypeLabel(item.row.type) +
          (item.row.default_group ? ' / ' + item.row.default_group : ''),
      }));
    }

    function populateReferenceControls() {
      const ref = state.reference || {};
      fillReferenceSelects('groups', (ref.groups || []).map(value => ({ value, label: value })));
      fillReferenceSelects('models', (ref.models || []).map(value => ({ value, label: value })));
      fillReferenceSelects('users', (ref.users || []).map(user => ({
        value: user.id,
        label: user.username + (user.display_name ? ' / ' + user.display_name : '') + ' (ID: ' + user.id + ') / ' + user.group,
      })), { numeric: true });
      fillReferenceSelects('org_units', organizationOptions(ref.org_units || []), { numeric: true });
      fillReferenceSelects('projects', (ref.org_units || []).filter(org => org.type === 'project').map(org => ({
        value: org.id,
        label: org.name + ' (ID: ' + org.id + ')',
      })), { numeric: true });
      fillReferenceSelects('cost_centers', (ref.org_units || []).filter(org => org.type === 'cost_center').map(org => ({
        value: org.id,
        label: org.name + ' (ID: ' + org.id + ')',
      })), { numeric: true });
      fillReferenceSelects('policies', (ref.policies || []).map(policy => ({
        value: policy.id,
        label: policy.name + ' (ID: ' + policy.id + ')' + (policy.default_group ? ' / ' + policy.default_group : ''),
      })), { numeric: true });
      fillReferenceSelects('enterprise_keys', (ref.enterprise_keys || []).map(key => ({
        value: key.id,
        label: key.name + ' (ID: ' + key.id + ')',
      })), { numeric: true });
      fillReferenceSelects('channels', (ref.channels || []).map(channel => ({
        value: channel.id,
        label: (channel.name || t('渠道')) + ' (ID: ' + channel.id + ')',
      })), { numeric: true });
      for (const id of ['budget-timezone', 'usage-timezone']) {
        const timezone = document.getElementById(id);
        if (timezone) timezone.textContent = ref.budget_timezone || 'UTC';
      }
      updateBudgetScopeOptions();
      updateAdminUsername();
      initializeMoneyQuotaFields();
    }

    function fillReferenceSelects(name, options, config) {
      document.querySelectorAll('select[data-ref-select="' + name + '"]').forEach(select => {
        fillSelect(select, options, Object.assign({}, config || {}, {
          multiple: select.multiple,
          placeholder: select.dataset.placeholder || '',
        }));
      });
    }

    function fillSelect(select, options, config) {
      const selected = new Set(Array.from(select.selectedOptions || []).map(option => option.value));
      const previous = select.value;
      const isMultiple = Boolean(config && config.multiple);
      select.innerHTML = '';
      if (!isMultiple) {
        const placeholder = document.createElement('option');
        placeholder.value = '';
        placeholder.textContent = config && config.placeholder ? config.placeholder : t('请选择');
        select.appendChild(placeholder);
      }
      for (const item of options || []) {
        const option = document.createElement('option');
        option.value = String(item.value);
        option.textContent = item.label;
        if (isMultiple ? selected.has(option.value) : previous === option.value) {
          option.selected = true;
        }
        select.appendChild(option);
      }
      if (config && config.numeric) {
        select.dataset.numeric = 'true';
      }
    }

    function referenceItem(collection, id) {
      const numericID = Number(id || 0);
      if (!numericID) return null;
      return (((state.reference || {})[collection]) || []).find(item => Number(item.id) === numericID) || null;
    }

    function namedID(collection, id, fallback) {
      const numericID = Number(id || 0);
      if (!numericID) return '';
      const item = referenceItem(collection, numericID);
      const name = item && (item.name || item.username || item.display_name);
      return (name || fallback || '#') + (name || fallback ? ' (ID: ' + numericID + ')' : numericID);
    }

    function recordID(value) {
      const id = Number(value || 0);
      return id ? '#' + id : '';
    }

    function policyIDs(value) {
      return (Array.isArray(value) ? value : []).map(id => namedID('policies', id, 'Policy')).join(', ');
    }

    function scopeName(type, id) {
      if (type === 'enterprise_key') return namedID('enterprise_keys', id, t('企业令牌'));
      return namedID('org_units', id, t(type || '组织'));
    }

    function translatedValue(value) {
      if (value === null || value === undefined || value === '') return '';
      return t(String(value));
    }

    function orgTypeLabel(value) {
      if (state.language === 'en') return t(String(value || ''));
      return ({
        company: '公司', business_unit: '事业部', department: '部门', team: '团队',
        project: '项目', cost_center: '成本中心'
      })[value] || String(value || '');
    }

    function orgStatusLabel(value) {
      if (state.language === 'en') return t(String(value || ''));
      return ({ enabled: '启用', disabled: '禁用' })[value] || String(value || '');
    }

    function updateBudgetScopeOptions() {
      const form = document.getElementById('budget-form');
      if (!form) return;
      const type = form.elements.scope_type.value;
      const ref = state.reference || {};
      let options = [];
      if (type === 'enterprise_key') {
        options = (ref.enterprise_keys || []).map(key => ({
          value: key.id,
          label: key.name + ' (ID: ' + key.id + ') / token ' + (key.newapi_token_id ? '#' + key.newapi_token_id : '-'),
        }));
      } else {
        const orgType = type === 'org_unit' ? '' : type;
        options = (ref.org_units || [])
          .filter(org => !orgType || org.type === orgType)
          .map(org => ({ value: org.id, label: org.name + ' (ID: ' + org.id + ') / ' + t(org.type) }));
      }
      fillSelect(document.getElementById('budget-scope-id'), options, { numeric: true, placeholder: t('请选择范围对象') });
    }

    function updateAdminUsername() {
      const select = document.getElementById('admin-user-select');
      const input = document.getElementById('admin-username-input');
      if (!select || !input) return;
      const users = (state.reference && state.reference.users) || [];
      const user = users.find(item => String(item.id) === String(select.value));
      input.value = user ? user.username : '';
    }

    function setFormValue(form, name, value) {
      const input = form.elements[name];
      if (!input) return;
      if (input.type === 'checkbox') {
        input.checked = Boolean(value);
        return;
      }
      if (input.tagName === 'SELECT' && input.multiple) {
        const values = new Set((Array.isArray(value) ? value : []).map(String));
        for (const selectedValue of values) {
          if (!Array.from(input.options).some(option => option.value === selectedValue)) {
            const option = document.createElement('option');
            option.value = selectedValue;
            option.textContent = selectedValue + (state.language === 'en' ? ' (configured)' : '（已配置）');
            input.appendChild(option);
          }
        }
        Array.from(input.options).forEach(option => { option.selected = values.has(option.value); });
        return;
      }
      input.value = value === null || value === undefined ? '' : String(value);
    }

    function orgRow(id) {
      const numericID = Number(id || 0);
      return (state.orgs || []).find(item => Number(item.id) === numericID) || null;
    }

    function policyRow(id) {
      const numericID = Number(id || 0);
      return (state.policies || []).find(item => Number(item.id) === numericID) || referenceItem('policies', numericID);
    }

    function orgChain(row) {
      const chain = [];
      const visited = new Set();
      let current = row;
      while (current && !visited.has(Number(current.id))) {
        visited.add(Number(current.id));
        chain.unshift(current);
        current = orgRow(current.parent_id);
      }
      return chain;
    }

    function orgBreadcrumb(row) {
      return row ? orgChain(row).map(item => item.name).join(' / ') : t('顶层组织');
    }

    function effectiveOrgConfiguration(row) {
      const policies = [];
      const seen = new Set();
      let effectiveGroup = '';
      let groupSource = null;
      let monthlyBudget = 0;
      let dailyBudget = 0;
      let monthlyBudgetPolicy = null;
      let dailyBudgetPolicy = null;
      for (const org of orgChain(row)) {
        const policy = policyRow(org.default_policy_id);
        if (!policy || policy.status !== 'enabled' || seen.has(Number(policy.id))) continue;
        seen.add(Number(policy.id));
        policies.push({ policy, org });
        if (policy.default_group) {
          effectiveGroup = policy.default_group;
          groupSource = { kind: 'policy', name: policy.name, org };
        }
        const monthly = Number(policy.monthly_budget_quota || 0);
        const daily = Number(policy.daily_budget_quota || 0);
        if (monthly > 0 && (!monthlyBudget || monthly < monthlyBudget)) {
          monthlyBudget = monthly;
          monthlyBudgetPolicy = policy;
        }
        if (daily > 0 && (!dailyBudget || daily < dailyBudget)) {
          dailyBudget = daily;
          dailyBudgetPolicy = policy;
        }
      }
      if (!effectiveGroup && row.default_group) {
        effectiveGroup = row.default_group;
        groupSource = { kind: 'org', name: row.name, org: row };
      }
      return {
        policies,
        effectiveGroup,
        groupSource,
        monthlyBudget,
        dailyBudget,
        monthlyBudgetPolicy,
        dailyBudgetPolicy,
        account: referenceItem('users', row.newapi_user_id),
      };
    }

    function renderOrgDrawerSummary(row) {
      const summary = document.getElementById('org-effective-summary');
      if (!row) {
        summary.hidden = true;
        summary.innerHTML = '';
        return;
      }
      const effective = effectiveOrgConfiguration(row);
      const policyText = effective.policies.length
        ? effective.policies.map(item => item.policy.name).join(' ∩ ')
        : t('未设置');
      const policySource = effective.policies.length > 1 ? t('Policy 交集') :
        effective.policies.length === 1 ? effective.policies[0].org.name : '';
      const groupSource = effective.groupSource
        ? effective.groupSource.kind === 'policy'
          ? t('由 Policy {name} 设置', { name: effective.groupSource.name })
          : t('本级备用值')
        : '';
      const accountName = effective.account
        ? (effective.account.username || effective.account.display_name) + ' (ID: ' + effective.account.id + ')'
        : t('未设置');
      const budgetText = (effective.monthlyBudget || effective.dailyBudget)
        ? t('月预算') + ': ' + formatQuotaWithMoney(effective.monthlyBudget,
            effective.monthlyBudgetPolicy && effective.monthlyBudgetPolicy.monthly_budget_amount,
            effective.monthlyBudgetPolicy && effective.monthlyBudgetPolicy.monthly_budget_currency,
            effective.monthlyBudgetPolicy && effective.monthlyBudgetPolicy.monthly_budget_quota_per_unit,
            effective.monthlyBudgetPolicy && effective.monthlyBudgetPolicy.monthly_budget_exchange_rate) + ' / ' +
          t('日预算') + ': ' + formatQuotaWithMoney(effective.dailyBudget,
            effective.dailyBudgetPolicy && effective.dailyBudgetPolicy.daily_budget_amount,
            effective.dailyBudgetPolicy && effective.dailyBudgetPolicy.daily_budget_currency,
            effective.dailyBudgetPolicy && effective.dailyBudgetPolicy.daily_budget_quota_per_unit,
            effective.dailyBudgetPolicy && effective.dailyBudgetPolicy.daily_budget_exchange_rate)
        : t('未设置');
      summary.hidden = false;
      summary.innerHTML =
        '<div class="org-effective-item"><div class="label">' + escapeHTML(t('生效 Policy 链')) + '</div><div class="value">' + escapeHTML(policyText) + '</div>' +
          (policySource ? '<div class="org-cell-source">' + escapeHTML(policySource) + '</div>' : '') + '</div>' +
        '<div class="org-effective-item"><div class="label">' + escapeHTML(t('生效 group')) + '</div><div class="value">' + escapeHTML(effective.effectiveGroup || t('未设置')) + '</div>' +
          (groupSource ? '<div class="org-cell-source">' + escapeHTML(groupSource) + '</div>' : '') + '</div>' +
        '<div class="org-effective-item"><div class="label">' + escapeHTML(t('所属账号')) + '</div><div class="value">' + escapeHTML(accountName) + '</div></div>' +
        '<div class="org-effective-item"><div class="label">' + escapeHTML(t('预算')) + '</div><div class="value">' + escapeHTML(budgetText) + '</div></div>';
    }

    function openOrgDrawer() {
      const drawer = document.getElementById('org-drawer');
      drawer.classList.add('open');
      drawer.setAttribute('aria-hidden', 'false');
      window.setTimeout(() => document.querySelector('#org-form input[name="name"]').focus(), 0);
    }

    function closeOrgDrawer() {
      const drawer = document.getElementById('org-drawer');
      drawer.classList.remove('open');
      drawer.setAttribute('aria-hidden', 'true');
      resetOrgForm();
    }

    function handleOrgDrawerBackdrop(event) {
      if (event.target === document.getElementById('org-drawer')) closeOrgDrawer();
    }

    function beginOrgCreate(parentID) {
      switchTab('orgs');
      resetOrgForm();
      const parent = orgRow(parentID);
      const form = document.getElementById('org-form');
      setFormValue(form, 'parent_id', parent ? parent.id : 0);
      document.getElementById('org-form-title').textContent = parent ? t('创建下级组织') : t('创建顶层组织');
      document.getElementById('org-breadcrumb').textContent = parent
        ? orgBreadcrumb(parent) + ' / ' + t('创建')
        : t('顶层组织');
      openOrgDrawer();
    }

    function beginOrgEdit(id) {
      const row = orgRow(id);
      if (!row) return;
      switchTab('orgs');
      state.editingOrgId = id;
      state.selectedOrgId = id;
      const form = document.getElementById('org-form');
      ['name', 'code', 'parent_id', 'type', 'default_policy_id', 'default_group', 'newapi_user_id', 'status']
        .forEach(name => setFormValue(form, name, row[name]));
      form.elements.parent_id.disabled = true;
      document.getElementById('org-form-title').textContent = t('编辑') + ' ' + row.name + ' #' + id;
      document.getElementById('org-breadcrumb').textContent = orgBreadcrumb(row);
      document.getElementById('org-submit').textContent = t('保存修改');
      renderOrgDrawerSummary(row);
      renderOrgTree();
      openOrgDrawer();
    }

    function beginTypedOrgCreate(type) {
      beginOrgCreate(0);
      setFormValue(document.getElementById('org-form'), 'type', type);
      document.getElementById('org-form-title').textContent = t('创建') + ' ' + t(type);
    }

    function resetOrgForm() {
      state.editingOrgId = 0;
      const form = document.getElementById('org-form');
      form.reset();
      form.elements.parent_id.disabled = false;
      document.getElementById('org-form-title').textContent = t('创建组织节点');
      document.getElementById('org-breadcrumb').textContent = t('顶层组织');
      document.getElementById('org-submit').textContent = t('创建');
      renderOrgDrawerSummary(null);
    }

    function beginPolicyEdit(id) {
      const row = (state.policies || []).find(item => item.id === id);
      if (!row) return;
      state.editingPolicyId = id;
      const form = document.getElementById('policy-form');
      ['name', 'default_group', 'status']
        .forEach(name => setFormValue(form, name, row[name]));
      ['monthly_budget', 'daily_budget', 'key_default'].forEach(prefix => setMoneyQuotaField(form, prefix, row));
      setFormValue(form, 'allowed_models', row.allowed_models_list || []);
      setFormValue(form, 'denied_models', row.denied_models_list || []);
      document.getElementById('policy-form-title').textContent = t('编辑') + ' Policy #' + id;
      document.getElementById('policy-submit').textContent = t('保存修改');
      document.getElementById('policy-cancel-edit').hidden = false;
      form.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function resetPolicyForm() {
      state.editingPolicyId = 0;
      const form = document.getElementById('policy-form');
      form.reset();
      resetMoneyQuotaFields(form);
      document.getElementById('policy-form-title').textContent = t('创建 Policy');
      document.getElementById('policy-submit').textContent = t('创建');
      document.getElementById('policy-cancel-edit').hidden = true;
    }

    function beginKeyEdit(id) {
      const row = (state.keys || []).find(item => item.id === id);
      if (!row) return;
      switchTab('keys');
      state.editingKeyId = id;
      const form = document.getElementById('key-form');
      ['name', 'org_unit_id', 'project_id', 'cost_center_id', 'policy_id',
        'environment', 'purpose', 'contact', 'status'].forEach(name => setFormValue(form, name, row[name]));
      setFormValue(form, 'newapi_user_id', row.configured_newapi_user_id || 0);
      document.getElementById('key-form-title').textContent = t('编辑') + ' ' + t('企业令牌') + ' #' + id;
      document.getElementById('key-submit').textContent = t('保存并同步');
      document.getElementById('key-cancel-edit').hidden = false;
      form.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function resetKeyForm() {
      state.editingKeyId = 0;
      const form = document.getElementById('key-form');
      form.reset();
      document.getElementById('key-form-title').textContent = t('创建企业令牌');
      document.getElementById('key-submit').textContent = t('创建并同步');
      document.getElementById('key-cancel-edit').hidden = true;
    }

    function beginBudgetEdit(id) {
      const row = (state.budgets || []).find(item => item.id === id);
      if (!row || row.source_type === 'policy') return;
      state.editingBudgetId = id;
      const form = document.getElementById('budget-form');
      ['scope_type', 'scope_id', 'status'].forEach(name => setFormValue(form, name, row[name]));
      updateBudgetScopeOptions();
      setFormValue(form, 'scope_id', row.scope_id);
      setMoneyQuotaField(form, 'budget', row);
      setFormValue(form, 'period_start', unixSecondsToLocalInput(row.period_start));
      setFormValue(form, 'period_end', unixSecondsToLocalInput(row.period_end));
      document.getElementById('budget-form-title').textContent = t('编辑') + ' ' + t('手工预算') + ' #' + id;
      document.getElementById('budget-submit').textContent = t('保存修改');
      document.getElementById('budget-cancel-edit').hidden = false;
      form.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function resetBudgetForm() {
      state.editingBudgetId = 0;
      const form = document.getElementById('budget-form');
      form.reset();
      updateBudgetScopeOptions();
      resetMoneyQuotaFields(form);
      document.getElementById('budget-form-title').textContent = t('创建手工预算');
      document.getElementById('budget-submit').textContent = t('创建');
      document.getElementById('budget-cancel-edit').hidden = true;
    }

    async function deleteOrg(id) {
      const row = (state.orgs || []).find(item => item.id === id);
      const message = state.language === 'en'
        ? 'Delete organization “' + (row ? row.name : '#' + id) + '”? Referenced organizations cannot be deleted.'
        : '确认删除组织“' + (row ? row.name : '#' + id) + '”？被引用的组织不会被删除。';
      if (!window.confirm(message)) return;
      try {
        await api('org-units/' + id, { method: 'DELETE' });
        if (state.editingOrgId === id) closeOrgDrawer();
        if (state.selectedOrgId === id) state.selectedOrgId = 0;
        delete state.orgExpanded[id];
        await Promise.all([loadReference(), loadOrgs()]);
      } catch (error) { showError(error); }
    }

    async function deletePolicy(id) {
      const row = (state.policies || []).find(item => item.id === id);
      const message = state.language === 'en'
        ? 'Delete Policy “' + (row ? row.name : '#' + id) + '”? Referenced Policies cannot be deleted.'
        : '确认删除 Policy“' + (row ? row.name : '#' + id) + '”？被引用的 Policy 不会被删除。';
      if (!window.confirm(message)) return;
      try {
        await api('policies/' + id, { method: 'DELETE' });
        if (state.editingPolicyId === id) resetPolicyForm();
        await Promise.all([loadReference(), loadPolicies(), loadOrgs()]);
      } catch (error) { showError(error); }
    }

    async function deleteKey(id) {
      const row = (state.keys || []).find(item => item.id === id);
      const name = row ? row.name : '#' + id;
      const message = state.language === 'en'
        ? 'Delete enterprise token “' + name + '”? Its new-api token will be revoked immediately; historical usage is retained.'
        : '确认删除企业令牌“' + name + '”？对应的 new-api Token 会立即撤销，历史用量仍保留。';
      if (!window.confirm(message)) return;
      try {
        await api('keys/' + id, { method: 'DELETE' });
        if (state.editingKeyId === id) resetKeyForm();
        await Promise.all([loadReference(), loadKeys(), loadUsage(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    async function resetBudget(id) {
      if (!window.confirm(state.language === 'en' ? 'Reset this budget usage to zero? Token blocks caused by it will be released.' : '确认把该预算的已用额度归零？由该预算造成的令牌阻断会被释放。')) return;
      try {
        await api('budgets/' + id + '/reset', { method: 'POST', body: '{}' });
        await Promise.all([loadBudgets(), loadKeys(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    async function deleteBudget(id) {
      if (!window.confirm(state.language === 'en' ? 'Delete this manual budget? Token blocks caused by it will be released.' : '确认删除该手工预算？由该预算造成的令牌阻断会被释放。')) return;
      try {
        await api('budgets/' + id, { method: 'DELETE' });
        if (state.editingBudgetId === id) resetBudgetForm();
        await Promise.all([loadBudgets(), loadKeys(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    function table(rows, columns, actions) {
      if (!rows || rows.length === 0) return '<div class="status">' + escapeHTML(t('暂无数据')) + '</div>';
      const head = columns.map(col => '<th class="' + (col.className || '') + '">' + escapeHTML(t(col.label)) + '</th>').join('') +
        (actions ? '<th class="actions">' + escapeHTML(t('操作')) + '</th>' : '');
      const body = rows.map(row => {
        const cells = columns.map(col => {
          const value = col.format ? col.format(row[col.key], row) : formatValue(row[col.key]);
          return '<td class="' + (col.className || '') + '">' + escapeHTML(formatValue(value)) + '</td>';
        }).join('');
        const actionCell = actions ? '<td class="actions">' + actions(row) + '</td>' : '';
        return '<tr>' + cells + actionCell + '</tr>';
      }).join('');
      return '<div class="table-scroll"><table><thead><tr>' + head + '</tr></thead><tbody>' + body + '</tbody></table></div>';
    }

    function formatValue(value) {
      if (value === null || value === undefined) return '';
      if (Array.isArray(value)) return value.join(', ');
      if (typeof value === 'object') return JSON.stringify(value);
      return String(value);
    }

    function formatUnixSeconds(value) {
      const seconds = Number(value || 0);
      if (!seconds) return t('不限');
      return new Date(seconds * 1000).toLocaleString(state.language === 'en' ? 'en-US' : 'zh-CN', {
        timeZone: budgetTimeZone(),
        hour12: false
      });
    }

    function unixSecondsToLocalInput(value) {
      const seconds = Number(value || 0);
      if (!seconds) return '';
      const parts = zonedDateParts(new Date(seconds * 1000));
      return parts.year + '-' + parts.month + '-' + parts.day + 'T' + parts.hour + ':' + parts.minute;
    }

    function budgetTimeZone() {
      return ((state.reference || {}).budget_timezone) || 'UTC';
    }

    function zonedDateParts(date) {
      const parts = new Intl.DateTimeFormat('en-CA', {
        timeZone: budgetTimeZone(), year: 'numeric', month: '2-digit', day: '2-digit',
        hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23'
      }).formatToParts(date);
      const values = {};
      for (const part of parts) {
        if (part.type !== 'literal') values[part.type] = part.value;
      }
      return values;
    }

    function zonedLocalToUnix(value) {
      const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(value);
      if (!match) return NaN;
      const intended = Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]), Number(match[4]), Number(match[5]), Number(match[6] || 0));
      let guess = intended;
      for (let attempt = 0; attempt < 3; attempt++) {
        const parts = zonedDateParts(new Date(guess));
        const observed = Date.UTC(Number(parts.year), Number(parts.month) - 1, Number(parts.day), Number(parts.hour), Number(parts.minute), Number(parts.second));
        const correction = intended - observed;
        guess += correction;
        if (correction === 0) break;
      }
      return Math.floor(guess / 1000);
    }

    function escapeHTML(value) {
      return value.replace(/[&<>"']/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
    }

    function showError(error) {
      alert(error.message || error);
    }

    function renderJSON(id, value) {
      document.getElementById(id).textContent = JSON.stringify(value || {}, null, 2);
    }

    function switchTab(tab) {
      document.querySelectorAll('nav button').forEach(button => button.classList.toggle('active', button.dataset.tab === tab));
      document.querySelectorAll('section').forEach(section => section.classList.toggle('active', section.id === tab));
      if (tab === 'usage') loadUsage();
      if (tab === 'audit') loadAudit();
      if (tab === 'tokenop') loadTokenOperation();
    }

    async function loadMe() {
      const me = await api('auth/me');
      state.me = me;
      document.getElementById('me').textContent = me.username + ' / ' + me.hub_role;
      renderJSON('me-json', { username: me.username, hub_role: me.hub_role, scope_org_unit_id: me.scope_org_unit_id || 0 });
    }

    function orgMatchesFilters(row) {
      const search = String(document.getElementById('org-search').value || '').trim().toLowerCase();
      const type = document.getElementById('org-type-filter').value;
      const status = document.getElementById('org-status-filter').value;
      const searchable = [row.name, row.code, row.id].join(' ').toLowerCase();
      return (!search || searchable.includes(search)) && (!type || row.type === type) && (!status || row.status === status);
    }

    function renderOrgTree() {
      const rows = state.orgs || [];
      const entries = organizationTreeRows(rows);
      if (!state.orgTreeInitialized) {
        for (const item of entries) {
          if (item.children.length) state.orgExpanded[item.row.id] = true;
        }
        state.orgTreeInitialized = true;
      }

      const search = String(document.getElementById('org-search').value || '').trim();
      const type = document.getElementById('org-type-filter').value;
      const status = document.getElementById('org-status-filter').value;
      const filtering = Boolean(search || type || status);
      const included = new Set();
      if (filtering) {
        for (const row of rows) {
          if (!orgMatchesFilters(row)) continue;
          for (const ancestor of orgChain(row)) included.add(Number(ancestor.id));
        }
      }

      const visible = entries.filter(item => {
        const row = item.row;
        if (filtering) return included.has(Number(row.id));
        let parent = orgRow(row.parent_id);
        while (parent) {
          if (state.orgExpanded[parent.id] === false) return false;
          parent = orgRow(parent.parent_id);
        }
        return true;
      });

      const header = [t('组织节点'), t('类型'), t('生效 Policy 链'), t('生效 group'), t('所属账号'), t('状态'), t('操作')]
        .map(label => '<div>' + escapeHTML(label) + '</div>').join('');
      const body = visible.map(item => {
        const row = item.row;
        const effective = effectiveOrgConfiguration(row);
        const hasChildren = item.children.length > 0;
        const toggle = hasChildren
          ? '<button class="org-tree-toggle" type="button" aria-label="' + escapeHTML(t(state.orgExpanded[row.id] === false ? '展开' : '收起')) + '" aria-expanded="' +
            (state.orgExpanded[row.id] === false ? 'false' : 'true') + '" onclick="toggleOrgNode(' + row.id + ')">›</button>'
          : '<span class="org-tree-spacer"></span>';
        const policyText = effective.policies.length
          ? effective.policies.map(entry => entry.policy.name).join(' ∩ ')
          : t('未设置');
        const policySource = effective.policies.length
          ? effective.policies.map(entry => entry.org.name).join(' → ')
          : '';
        let groupSource = '';
        if (effective.groupSource) {
          groupSource = effective.groupSource.kind === 'policy'
            ? t('由 Policy {name} 设置', { name: effective.groupSource.name })
            : t('直接配置');
        }
        const accountName = effective.account
          ? (effective.account.username || effective.account.display_name || ('#' + effective.account.id)) + ' (ID: ' + effective.account.id + ')'
          : t('未设置');
        const selected = Number(state.selectedOrgId) === Number(row.id) ? ' selected' : '';
        return '<div class="org-tree-row' + selected + '">' +
          '<div><div class="org-node-cell"><span class="org-node-indent" style="--org-depth:' + item.depth + '"></span>' + toggle +
            '<div class="org-node-main"><button class="org-name-button" type="button" onclick="beginOrgEdit(' + row.id + ')">' + escapeHTML(row.name) + '</button>' +
            '<div class="org-node-path">' + escapeHTML(orgBreadcrumb(row)) + (row.code ? ' · ' + escapeHTML(row.code) : '') + ' · #' + row.id + '</div></div></div></div>' +
          '<div><span class="org-type-badge">' + escapeHTML(orgTypeLabel(row.type)) + '</span></div>' +
          '<div>' + escapeHTML(policyText) + (policySource ? '<div class="org-cell-source">' + escapeHTML(policySource) + '</div>' : '') + '</div>' +
          '<div>' + escapeHTML(effective.effectiveGroup || t('未设置')) + (groupSource ? '<div class="org-cell-source">' + escapeHTML(groupSource) + '</div>' : '') + '</div>' +
          '<div>' + escapeHTML(accountName) + (effective.account ? '<span class="org-source-badge">' + escapeHTML(t('直接配置')) + '</span>' : '') + '</div>' +
          '<div><span class="org-status-dot ' + escapeHTML(row.status) + '"></span>' + escapeHTML(orgStatusLabel(row.status)) + '</div>' +
          '<div><div class="row-actions"><button class="secondary" type="button" onclick="beginOrgCreate(' + row.id + ')">' + escapeHTML(t('添加下级')) + '</button>' +
            '<button class="secondary" type="button" onclick="beginOrgEdit(' + row.id + ')">' + escapeHTML(t('编辑')) + '</button>' +
            '<button class="danger" type="button" onclick="deleteOrg(' + row.id + ')">' + escapeHTML(t('删除')) + '</button></div></div>' +
          '</div>';
      }).join('');

      document.getElementById('org-tree').innerHTML = body
        ? '<div class="org-tree-scroll"><div class="org-tree-grid"><div class="org-tree-header">' + header + '</div>' + body + '</div></div>'
        : '<div class="org-tree-empty">' + escapeHTML(t('没有符合条件的组织节点')) + '</div>';
    }

    function toggleOrgNode(id) {
      state.orgExpanded[id] = state.orgExpanded[id] === false;
      renderOrgTree();
    }

    function expandAllOrgs() {
      for (const row of state.orgs || []) state.orgExpanded[row.id] = true;
      renderOrgTree();
    }

    function collapseAllOrgs() {
      for (const row of state.orgs || []) state.orgExpanded[row.id] = false;
      renderOrgTree();
    }

    async function loadOrgs() {
      const rows = await api('org-units');
      state.orgs = rows;
      renderOrgTree();
    }

    async function loadPolicies() {
      const rows = await api('policies');
      state.policies = rows;
      document.getElementById('policy-table').innerHTML = table(rows, [
        { key: 'id', label: 'ID', className: 'cell-compact', format: recordID },
        { key: 'name', label: '名称', className: 'cell-medium' }, { key: 'default_group', label: 'group', className: 'cell-medium' },
        { key: 'allowed_models_list', label: '允许模型', className: 'cell-wide' },
        { key: 'denied_models_list', label: '禁止模型', className: 'cell-wide' },
        { key: 'monthly_budget_quota', label: '月预算', className: 'cell-medium', format: (value, row) => formatQuotaWithMoney(value, row.monthly_budget_amount, row.monthly_budget_currency, row.monthly_budget_quota_per_unit, row.monthly_budget_exchange_rate) },
        { key: 'daily_budget_quota', label: '日预算', className: 'cell-medium', format: (value, row) => formatQuotaWithMoney(value, row.daily_budget_amount, row.daily_budget_currency, row.daily_budget_quota_per_unit, row.daily_budget_exchange_rate) },
        { key: 'key_default_quota', label: 'Key quota', className: 'cell-medium', format: (value, row) => formatQuotaWithMoney(value, row.key_default_amount, row.key_default_currency, row.key_default_quota_per_unit, row.key_default_exchange_rate) },
        { key: 'status', label: '状态', format: translatedValue }
      ], row => '<div class="row-actions"><button class="secondary" onclick="beginPolicyEdit(' + row.id + ')">' + t('编辑') + '</button>' +
        '<button class="danger" onclick="deletePolicy(' + row.id + ')">' + t('删除') + '</button></div>');
      if (state.orgs) renderOrgTree();
    }

    async function loadKeys() {
      const rows = await api('keys');
      state.keys = rows;
      document.getElementById('key-table').innerHTML = table(rows, [
        { key: 'id', label: 'ID', className: 'cell-compact', format: recordID },
        { key: 'name', label: '名称', className: 'cell-medium' },
        { key: 'org_unit_id', label: '组织', className: 'cell-medium', format: value => namedID('org_units', value, t('组织')) },
        { key: 'project_id', label: '项目', className: 'cell-medium', format: value => namedID('org_units', value, t('项目')) },
        { key: 'cost_center_id', label: '成本中心', className: 'cell-medium', format: value => namedID('org_units', value, t('成本中心')) },
        { key: 'policy_id', label: '直接 Policy', className: 'cell-medium', format: value => namedID('policies', value, 'Policy') },
        { key: 'effective_policy_ids', label: '有效 Policy', className: 'cell-wide', format: policyIDs },
        { key: 'newapi_user_id', label: '所属账号', className: 'cell-medium', format: value => namedID('users', value, t('所属账号')) },
        { key: 'newapi_token_id', label: 'new-api token', className: 'cell-medium', format: (value, row) => value ? (row.new_api_token_name || row.name || 'token') + ' (ID: ' + value + ')' : '' },
        { key: 'key_fingerprint', label: '指纹', className: 'cell-medium' },
        { key: 'status', label: '管理员状态', format: translatedValue }, { key: 'effective_status', label: '生效状态', format: translatedValue },
        { key: 'applied_key_quota', label: '令牌额度', className: 'cell-medium', format: value => formatQuotaWithMoney(value, '', normalizedDisplayCurrency()) },
        { key: 'active_budget_blocks', label: '预算阻断' }, { key: 'sync_status', label: '同步', format: translatedValue }
      ], row => '<div class="row-actions"><button class="secondary" onclick="beginKeyEdit(' + row.id + ')">' + t('编辑') + '</button>' +
        '<button class="secondary" onclick="syncKey(' + row.id + ')">' + t('同步') + '</button>' +
        '<button class="secondary" onclick="rotateKey(' + row.id + ')">' + t('轮换') + '</button>' +
        '<button class="danger" onclick="setKeyStatus(' + row.id + ', \'disable\')">' + t('禁用') + '</button>' +
        '<button class="secondary" onclick="setKeyStatus(' + row.id + ', \'enable\')">' + t('启用') + '</button>' +
        '<button class="danger" onclick="deleteKey(' + row.id + ')">' + t('删除') + '</button></div>');
    }

    async function loadBudgets() {
      const rows = await api('budgets');
      state.budgets = rows;
      document.getElementById('budget-table').innerHTML = table(rows, [
        { key: 'id', label: 'ID', className: 'cell-compact', format: recordID },
        { key: 'scope_type', label: '范围', format: translatedValue },
        { key: 'scope_id', label: '范围对象', className: 'cell-medium', format: (value, row) => scopeName(row.scope_type, value) },
        { key: 'source_type', label: '来源', format: translatedValue },
        { key: 'source_id', label: 'Policy', className: 'cell-medium', format: (value, row) => row.source_type === 'policy' ? namedID('policies', value, 'Policy') : recordID(value) },
        { key: 'period_kind', label: '周期', format: translatedValue },
        { key: 'period_start', label: '开始时间', format: formatUnixSeconds },
        { key: 'period_end', label: '结束时间', format: formatUnixSeconds },
        { key: 'budget_quota', label: '预算', className: 'cell-medium', format: (value, row) => formatQuotaWithMoney(value, row.budget_amount, row.budget_currency, row.budget_quota_per_unit, row.budget_exchange_rate) },
        { key: 'confirmed_used_quota', label: '已结算', className: 'cell-medium', format: (value, row) => formatQuotaWithMoney(value, '', row.budget_currency, row.budget_quota_per_unit, row.budget_exchange_rate) },
        { key: 'active_block_count', label: '阻断令牌' }, { key: 'status', label: '状态', format: translatedValue }
      ], row => row.source_type === 'policy'
        ? '<span class="status">' + t('通过 Policy 管理') + '</span>'
        : '<div class="row-actions"><button class="secondary" onclick="beginBudgetEdit(' + row.id + ')">' + t('编辑') + '</button>' +
          '<button class="secondary" onclick="resetBudget(' + row.id + ')">' + t('归零') + '</button>' +
          '<button class="danger" onclick="deleteBudget(' + row.id + ')">' + t('删除') + '</button></div>');
    }

    function usageQuery(includeGroup) {
      const params = new URLSearchParams();
      if (includeGroup) params.set('group_by', document.getElementById('usage-group').value);
      const selectFilters = {
        'usage-org': 'org_unit_id',
        'usage-key': 'enterprise_key_id',
        'usage-model': 'model_name',
        'usage-channel': 'channel_id',
        'usage-project': 'project_id',
        'usage-cost-center': 'cost_center_id'
      };
      for (const elementID of Object.keys(selectFilters)) {
        const value = document.getElementById(elementID).value;
        if (value) params.set(selectFilters[elementID], value);
      }
      const start = document.getElementById('usage-start').value;
      const end = document.getElementById('usage-end').value;
      if (start) params.set('created_at_start', String(zonedLocalToUnix(start)));
      if (end) params.set('created_at_end', String(zonedLocalToUnix(end)));
      return params;
    }

    function usageDimension(value) {
      const groupBy = document.getElementById('usage-group').value;
      if (value === '(empty)' || value === '' || value === '0') return state.language === 'en' ? '(Unassigned)' : '（未分配）';
      if (groupBy === 'org_unit' || groupBy === 'project' || groupBy === 'cost_center') {
        return namedID('org_units', value, t(groupBy));
      }
      if (groupBy === 'key') return namedID('enterprise_keys', value, t('企业令牌'));
      if (groupBy === 'channel') return namedID('channels', value, t('渠道'));
      return value;
    }

    function resetUsageFilters() {
      ['usage-org', 'usage-key', 'usage-model', 'usage-channel', 'usage-project', 'usage-cost-center', 'usage-start', 'usage-end']
        .forEach(id => { document.getElementById(id).value = ''; });
      loadUsage();
    }

    async function loadUsage() {
      const summaryParams = usageQuery(true);
      const detailParams = usageQuery(false);
      detailParams.set('limit', '50');
      const [summary, details] = await Promise.all([
        api('usage/summary?' + summaryParams.toString()),
        api('usage/details?' + detailParams.toString())
      ]);
      document.getElementById('usage-summary').innerHTML = table(summary, [
        { key: 'key', label: '维度', className: 'cell-wide', format: usageDimension },
        { key: 'confirmed_amount', label: '已结算', className: 'cell-medium', format: (value, row) => formatUSDInDisplayCurrency(value) + ' · ' + Number(row.confirmed_quota || 0).toLocaleString() + ' quota' },
        { key: 'pending_amount', label: '待结算', className: 'cell-medium', format: (value, row) => formatUSDInDisplayCurrency(value) + ' · ' + Number(row.pending_quota || 0).toLocaleString() + ' quota' },
        { key: 'amount', label: '合计', className: 'cell-medium', format: (value, row) => formatUSDInDisplayCurrency(value) + ' · ' + Number(row.quota || 0).toLocaleString() + ' quota' },
        { key: 'count', label: '记录数' }
      ]);
      document.getElementById('usage-details').innerHTML = table(details, [
        { key: 'id', label: 'ID', className: 'cell-compact', format: recordID },
        { key: 'newapi_log_id', label: 'log', className: 'cell-compact', format: recordID },
        { key: 'enterprise_key_id', label: '企业令牌', className: 'cell-medium', format: value => namedID('enterprise_keys', value, t('企业令牌')) },
        { key: 'org_unit_id', label: '组织', className: 'cell-medium', format: value => namedID('org_units', value, t('组织')) },
        { key: 'project_id', label: '项目', className: 'cell-medium', format: value => namedID('org_units', value, t('项目')) },
        { key: 'cost_center_id', label: '成本中心', className: 'cell-medium', format: value => namedID('org_units', value, t('成本中心')) },
        { key: 'model_name', label: '模型', className: 'cell-wide' },
        { key: 'channel_id', label: '渠道', className: 'cell-medium', format: value => namedID('channels', value, t('渠道')) },
        { key: 'task_id', label: '任务 ID', className: 'cell-wide' },
        { key: 'usage_state', label: '结算状态', format: translatedValue },
        { key: 'amount', label: '合计', className: 'cell-medium', format: (value, row) => formatUSDInDisplayCurrency(value) + ' · ' + Number(row.quota || 0).toLocaleString() + ' quota' },
        { key: 'created_at', label: '时间', className: 'cell-medium', format: formatUnixSeconds }
      ]);
    }

    async function loadAudit() {
      const audits = await api('audit-logs?limit=50');
      const jobs = await api('sync-jobs?limit=50');
      document.getElementById('audit-table').innerHTML = table(audits, [
        { key: 'id', label: 'ID', className: 'cell-compact', format: recordID },
        { key: 'admin_newapi_user_id', label: '管理员', className: 'cell-medium', format: (value, row) => namedID('users', value, row.admin_username) },
        { key: 'action', label: '动作', className: 'cell-medium' }, { key: 'target_type', label: '对象', format: translatedValue },
        { key: 'target_id', label: '对象 ID', className: 'cell-medium', format: (value, row) => targetName(row.target_type, value) },
        { key: 'created_at', label: '时间', className: 'cell-medium', format: formatUnixSeconds }
      ]);
      document.getElementById('sync-table').innerHTML = table(jobs, [
        { key: 'id', label: 'ID', className: 'cell-compact', format: recordID },
        { key: 'entity_type', label: '实体', format: translatedValue },
        { key: 'entity_id', label: '实体 ID', className: 'cell-medium', format: (value, row) => targetName(row.entity_type, value) },
        { key: 'operation', label: '操作', format: translatedValue }, { key: 'status', label: '状态', format: translatedValue },
        { key: 'error_message', label: '错误', className: 'cell-wide' }
      ]);
    }

    async function loadAdmins() {
      const rows = await api('admin-bindings');
      document.getElementById('admin-table').innerHTML = table(rows, [
        { key: 'id', label: 'ID', className: 'cell-compact', format: recordID },
        { key: 'newapi_user_id', label: '用户列表', className: 'cell-medium', format: (value, row) => namedID('users', value, row.newapi_username) },
        { key: 'hub_role', label: 'Hub 角色', className: 'cell-medium' },
        { key: 'scope_org_unit_id', label: '组织范围', className: 'cell-medium', format: value => value ? namedID('org_units', value, t('组织')) : t('全局') },
        { key: 'status', label: '状态', format: translatedValue }
      ]);
    }

    function targetName(type, id) {
      if (type === 'org_unit' || type === 'organization') return namedID('org_units', id, t('组织'));
      if (type === 'policy') return namedID('policies', id, 'Policy');
      if (type === 'enterprise_key' || type === 'key' || type === 'token') return namedID('enterprise_keys', id, t('企业令牌'));
      if (type === 'channel') return namedID('channels', id, t('渠道'));
      if (type === 'user') return namedID('users', id, t('所属账号'));
      return recordID(id);
    }

    async function loadTokenOperation() {
      const status = await api('token-operation/status');
      renderJSON('tokenop-status', status);
    }

    async function syncTokenOperationObjects() {
      try {
        const result = await api('token-operation/sync-objects', { method: 'POST', body: '{}' });
        renderJSON('tokenop-result', result);
        await Promise.all([loadTokenOperation(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    async function loadTokenOperationUsageDetails() {
      try {
        const limit = document.getElementById('tokenop-usage-limit').value || '50';
        const result = await api('token-operation/usage-details?limit=' + encodeURIComponent(limit));
        renderJSON('tokenop-usage-details', result);
      } catch (error) { showError(error); }
    }

    async function syncUsage() {
      try {
        const result = await api('usage/sync', { method: 'POST', body: '{}' });
        renderJSON('sync-result', result);
        await Promise.all([loadUsage(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    async function syncKey(id) {
      try {
        const result = await api('keys/' + id + '/sync', { method: 'POST', body: '{}' });
        showFullKey(result.full_key);
        await Promise.all([loadKeys(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    async function rotateKey(id) {
      if (!confirm(state.language === 'en' ? 'The old token will stop working after rotation. Continue?' : '轮换后旧令牌会失效，继续吗？')) return;
      try {
        const result = await api('keys/' + id + '/rotate', { method: 'POST', body: '{}' });
        showFullKey(result.full_key);
        await Promise.all([loadKeys(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    async function setKeyStatus(id, action) {
      try {
        await api('keys/' + id + '/' + action, { method: 'POST', body: '{}' });
        await Promise.all([loadKeys(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    function showFullKey(key) {
      const box = document.getElementById('new-key');
      if (!key) return;
      box.style.display = 'block';
      box.innerHTML = escapeHTML(t('完整 Key 只展示一次：')) + '<br><code>' + escapeHTML(key) + '</code>';
    }

    async function refreshAll() {
      try {
        await loadMe();
        await loadReference();
        await Promise.all([loadOrgs(), loadPolicies(), loadKeys(), loadBudgets(), loadAdmins(), loadUsage(), loadAudit(), loadTokenOperation()]);
      } catch (error) {
        document.getElementById('me').innerHTML = '<span class="error">' + escapeHTML(error.message) + '</span>';
      }
    }

    document.querySelectorAll('nav button').forEach(button => button.addEventListener('click', () => switchTab(button.dataset.tab)));
    document.getElementById('org-search').addEventListener('input', renderOrgTree);
    ['org-type-filter', 'org-status-filter'].forEach(id => document.getElementById(id).addEventListener('change', renderOrgTree));
    document.addEventListener('keydown', event => {
      if (event.key === 'Escape' && document.getElementById('org-drawer').classList.contains('open')) closeOrgDrawer();
    });
    document.getElementById('usage-group').addEventListener('change', loadUsage);
    ['usage-org', 'usage-key', 'usage-model', 'usage-channel', 'usage-project', 'usage-cost-center']
      .forEach(id => document.getElementById(id).addEventListener('change', loadUsage));
    document.getElementById('language-select').addEventListener('change', event => {
      state.language = event.target.value === 'en' ? 'en' : 'zh';
      window.localStorage.setItem('eph-language', state.language);
      applyLanguage();
      refreshAll();
    });
    document.querySelector('#budget-form [name="scope_type"]').addEventListener('change', updateBudgetScopeOptions);
    document.getElementById('admin-user-select').addEventListener('change', updateAdminUsername);
    document.getElementById('org-form').addEventListener('submit', async event => {
      event.preventDefault();
      try {
        const id = state.editingOrgId;
        const current = id ? (state.orgs || []).find(item => item.id === id) : null;
        const payload = Object.assign({}, current || {}, formJSON(event.target));
        await api(id ? 'org-units/' + id : 'org-units', { method: id ? 'PUT' : 'POST', body: JSON.stringify(payload) });
        closeOrgDrawer();
        await Promise.all([loadReference(), loadOrgs()]);
      } catch (error) { showError(error); }
    });
    document.getElementById('policy-form').addEventListener('submit', async event => {
      event.preventDefault();
      try {
        const id = state.editingPolicyId;
        const current = id ? (state.policies || []).find(item => item.id === id) : null;
        const payload = Object.assign({}, current || {}, formJSON(event.target));
        await api(id ? 'policies/' + id : 'policies', { method: id ? 'PUT' : 'POST', body: JSON.stringify(payload) });
        resetPolicyForm();
        await Promise.all([loadReference(), loadPolicies(), loadOrgs()]);
      } catch (error) { showError(error); }
    });
    document.getElementById('key-form').addEventListener('submit', async event => {
      event.preventDefault();
      try {
        const id = state.editingKeyId;
        const current = id ? (state.keys || []).find(item => item.id === id) : null;
        const payload = Object.assign({}, current || {}, formJSON(event.target));
        if (id) {
          await api('keys/' + id, { method: 'PUT', body: JSON.stringify(payload) });
          const result = await api('keys/' + id + '/sync', { method: 'POST', body: '{}' });
          showFullKey(result.full_key);
        } else {
          const result = await api('keys', { method: 'POST', body: JSON.stringify(payload) });
          showFullKey(result.full_key);
        }
        resetKeyForm();
        await Promise.all([loadReference(), loadKeys(), loadAudit()]);
      } catch (error) { showError(error); }
    });
    document.getElementById('budget-form').addEventListener('submit', async event => {
      event.preventDefault();
      try {
        const id = state.editingBudgetId;
        const current = id ? (state.budgets || []).find(item => item.id === id) : null;
        const payload = Object.assign({}, current || {}, formJSON(event.target));
        await api(id ? 'budgets/' + id : 'budgets', { method: id ? 'PUT' : 'POST', body: JSON.stringify(payload) });
        resetBudgetForm();
        await Promise.all([loadBudgets(), loadKeys(), loadAudit()]);
      } catch (error) { showError(error); }
    });
    document.getElementById('admin-form').addEventListener('submit', async event => {
      event.preventDefault();
      try { await api('admin-bindings', { method: 'POST', body: JSON.stringify(formJSON(event.target)) }); event.target.reset(); await loadAdmins(); } catch (error) { showError(error); }
    });
    applyLanguage();
    refreshAll();
  </script>
</body>
</html>
`
