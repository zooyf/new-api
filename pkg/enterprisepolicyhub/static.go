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
      z-index: 2;
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
    table {
      width: 100%;
      border-collapse: collapse;
      margin-top: 12px;
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
    }
    th, td {
      border-bottom: 1px solid var(--line);
      padding: 8px 10px;
      text-align: left;
      vertical-align: top;
      word-break: break-word;
    }
    th {
      background: #f8fafc;
      color: var(--muted);
      font-size: 12px;
      font-weight: 700;
    }
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
      <button class="secondary" onclick="refreshAll()">刷新</button>
      <button class="primary" onclick="syncUsage()">同步用量</button>
    </div>
  </header>
  <main>
    <nav>
      <button class="active" data-tab="overview">概览</button>
      <button data-tab="orgs">组织</button>
      <button data-tab="scopes">项目与成本中心</button>
      <button data-tab="policies">Policy</button>
      <button data-tab="keys">企业 Key</button>
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
        <h2>组织架构</h2>
        <div class="grid">
          <form class="panel" id="org-form">
            <h3 id="org-form-title">创建组织节点</h3>
            <label>名称 <input name="name" required></label>
            <label>编码 <input name="code"></label>
            <label>父级组织
              <select name="parent_id" data-ref-select="org_units" data-placeholder="顶层组织"></select>
              <span class="hint">从 Hub 已有组织读取；留空表示创建顶层组织。</span>
            </label>
            <label>类型
              <select name="type">
                <option value="department">department</option>
                <option value="company">company</option>
                <option value="business_unit">business_unit</option>
                <option value="team">team</option>
                <option value="project">project</option>
                <option value="cost_center">cost_center</option>
              </select>
            </label>
            <label>默认 Policy
              <select name="default_policy_id" data-ref-select="policies" data-placeholder="不绑定，按上级继承"></select>
            </label>
            <label>默认 new-api group
              <select name="default_group" data-ref-select="groups" data-placeholder="不覆盖"></select>
              <span class="hint">来自 new-api 分组配置、用户、渠道和 abilities。</span>
            </label>
            <label>new-api 服务账号
              <select name="newapi_user_id" data-ref-select="users" data-placeholder="继承或稍后指定"></select>
            </label>
            <label>状态
              <select name="status">
                <option value="enabled">启用</option>
                <option value="disabled">禁用</option>
              </select>
            </label>
            <div class="toolbar">
              <button class="primary" id="org-submit" type="submit">创建</button>
              <button class="secondary" id="org-cancel-edit" type="button" onclick="resetOrgForm()" hidden>取消编辑</button>
            </div>
          </form>
          <div class="panel">
            <h3>组织列表</h3>
            <div id="org-table"></div>
          </div>
        </div>
      </section>

      <section id="scopes">
        <h2>项目与成本中心</h2>
        <p class="status">项目和成本中心是组织树中的专用节点类型，可以继承上级组织的 Policy、group 和服务账号。</p>
        <div class="toolbar">
          <button class="primary" type="button" onclick="beginTypedOrgCreate('project')">创建项目</button>
          <button class="secondary" type="button" onclick="beginTypedOrgCreate('cost_center')">创建成本中心</button>
        </div>
        <div class="grid">
          <div class="panel"><h3>项目列表</h3><div id="project-table"></div></div>
          <div class="panel"><h3>成本中心列表</h3><div id="cost-center-table"></div></div>
        </div>
      </section>

      <section id="policies">
        <h2>Policy 管理</h2>
        <div class="grid">
          <form class="panel" id="policy-form">
            <h3 id="policy-form-title">创建 Policy</h3>
            <label>名称 <input name="name" required></label>
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
            <label>月预算 quota
              <input name="monthly_budget_quota" type="number">
              <span class="hint">按自然月限制该 Policy 所绑定组织或 Key 的合计用量，0 表示不限。</span>
            </label>
            <label>日预算 quota
              <input name="daily_budget_quota" type="number">
              <span class="hint">按预算时区的自然日自动创建周期，0 表示不限。</span>
            </label>
            <label>Key 默认 quota
              <input name="key_default_quota" type="number">
              <span class="hint">单个 Key 的总额度上限；重复同步不会重新充值。</span>
            </label>
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
        <h2>企业 Key</h2>
        <div class="grid">
          <form class="panel" id="key-form">
            <h3 id="key-form-title">创建企业 Key</h3>
            <label>名称 <input name="name" required></label>
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
            <label>new-api 服务账号
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
            <h3>Key 列表</h3>
            <div id="key-table"></div>
          </div>
        </div>
      </section>

      <section id="budgets">
        <h2>预算</h2>
        <p class="status">Policy 日/月预算按 <strong id="budget-timezone">预算时区</strong> 的自然周期执行；手工预算使用下方选择的精确时间范围。</p>
        <div class="grid">
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
            <label>范围对象
              <select name="scope_id" id="budget-scope-id" required></select>
              <span class="hint">会根据上面的范围类型自动切换组织、企业 Key、项目或成本中心。</span>
            </label>
            <label>开始时间
              <input name="period_start" type="datetime-local" data-unix-seconds="true">
              <span class="hint">留空表示立即生效，不限制开始时间。</span>
            </label>
            <label>结束时间
              <input name="period_end" type="datetime-local" data-unix-seconds="true">
              <span class="hint">留空表示长期有效，不限制结束时间。</span>
            </label>
            <label>预算 quota <input name="budget_quota" type="number" required></label>
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
        <div class="toolbar">
          <select id="usage-group">
            <option value="org_unit">按组织</option>
            <option value="key">按 Key</option>
            <option value="model">按模型</option>
            <option value="channel">按渠道</option>
            <option value="project">按项目</option>
            <option value="cost_center">按成本中心</option>
          </select>
          <button class="secondary" onclick="loadUsage()">刷新用量</button>
        </div>
        <div class="grid">
          <div class="panel"><h3>汇总</h3><div id="usage-summary"></div></div>
          <div class="panel"><h3>明细</h3><div id="usage-details"></div></div>
        </div>
      </section>

      <section id="admins">
        <h2>Hub 权限</h2>
        <div class="grid">
          <form class="panel" id="admin-form">
            <h3>授权 new-api 管理员</h3>
            <label>new-api 用户
              <select name="newapi_user_id" id="admin-user-select" data-ref-select="users" data-placeholder="选择管理员用户" required></select>
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
    const state = {};

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
        if (input.tagName === 'SELECT' && input.multiple) {
          data[input.name] = Array.from(input.selectedOptions).map(option => option.value).filter(Boolean);
          continue;
        }
        const trimmed = String(input.value || '').trim();
        if (input.dataset.unixSeconds === 'true') {
          const milliseconds = trimmed === '' ? 0 : new Date(trimmed).getTime();
          if (trimmed !== '' && !Number.isFinite(milliseconds)) {
            throw new Error('日期时间格式无效');
          }
          data[input.name] = milliseconds === 0 ? 0 : Math.floor(milliseconds / 1000);
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

    async function loadReference() {
      state.reference = await api('reference');
      populateReferenceControls();
    }

    function populateReferenceControls() {
      const ref = state.reference || {};
      fillReferenceSelects('groups', (ref.groups || []).map(value => ({ value, label: value })));
      fillReferenceSelects('models', (ref.models || []).map(value => ({ value, label: value })), { multiple: true });
      fillReferenceSelects('users', (ref.users || []).map(user => ({
        value: user.id,
        label: '#' + user.id + ' ' + user.username + (user.display_name ? ' / ' + user.display_name : '') + ' / ' + user.group,
      })), { numeric: true });
      fillReferenceSelects('org_units', (ref.org_units || []).map(org => ({
        value: org.id,
        label: '#' + org.id + ' ' + org.name + ' / ' + org.type + (org.default_group ? ' / ' + org.default_group : ''),
      })), { numeric: true });
      fillReferenceSelects('projects', (ref.org_units || []).filter(org => org.type === 'project').map(org => ({
        value: org.id,
        label: '#' + org.id + ' ' + org.name,
      })), { numeric: true });
      fillReferenceSelects('cost_centers', (ref.org_units || []).filter(org => org.type === 'cost_center').map(org => ({
        value: org.id,
        label: '#' + org.id + ' ' + org.name,
      })), { numeric: true });
      fillReferenceSelects('policies', (ref.policies || []).map(policy => ({
        value: policy.id,
        label: '#' + policy.id + ' ' + policy.name + (policy.default_group ? ' / ' + policy.default_group : ''),
      })), { numeric: true });
      const timezone = document.getElementById('budget-timezone');
      if (timezone) timezone.textContent = ref.budget_timezone || 'Asia/Shanghai';
      updateBudgetScopeOptions();
      updateAdminUsername();
    }

    function fillReferenceSelects(name, options, config) {
      document.querySelectorAll('select[data-ref-select="' + name + '"]').forEach(select => {
        fillSelect(select, options, Object.assign({}, config || {}, {
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
        placeholder.textContent = config && config.placeholder ? config.placeholder : '请选择';
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

    function updateBudgetScopeOptions() {
      const form = document.getElementById('budget-form');
      if (!form) return;
      const type = form.elements.scope_type.value;
      const ref = state.reference || {};
      let options = [];
      if (type === 'enterprise_key') {
        options = (ref.enterprise_keys || []).map(key => ({
          value: key.id,
          label: '#' + key.id + ' ' + key.name + ' / token ' + (key.newapi_token_id || '-'),
        }));
      } else {
        const orgType = type === 'org_unit' ? '' : type;
        options = (ref.org_units || [])
          .filter(org => !orgType || org.type === orgType)
          .map(org => ({ value: org.id, label: '#' + org.id + ' ' + org.name + ' / ' + org.type }));
      }
      fillSelect(document.getElementById('budget-scope-id'), options, { numeric: true, placeholder: '请选择范围对象' });
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
      if (input.tagName === 'SELECT' && input.multiple) {
        const values = new Set((Array.isArray(value) ? value : []).map(String));
        for (const selectedValue of values) {
          if (!Array.from(input.options).some(option => option.value === selectedValue)) {
            const option = document.createElement('option');
            option.value = selectedValue;
            option.textContent = selectedValue + '（已配置）';
            input.appendChild(option);
          }
        }
        Array.from(input.options).forEach(option => { option.selected = values.has(option.value); });
        return;
      }
      input.value = value === null || value === undefined ? '' : String(value);
    }

    function beginOrgEdit(id) {
      const row = (state.orgs || []).find(item => item.id === id);
      if (!row) return;
      switchTab('orgs');
      state.editingOrgId = id;
      const form = document.getElementById('org-form');
      ['name', 'code', 'parent_id', 'type', 'default_policy_id', 'default_group', 'newapi_user_id', 'status']
        .forEach(name => setFormValue(form, name, row[name]));
      form.elements.parent_id.disabled = true;
      document.getElementById('org-form-title').textContent = '编辑组织 #' + id;
      document.getElementById('org-submit').textContent = '保存修改';
      document.getElementById('org-cancel-edit').hidden = false;
      form.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function beginTypedOrgCreate(type) {
      switchTab('orgs');
      resetOrgForm();
      const form = document.getElementById('org-form');
      setFormValue(form, 'type', type);
      document.getElementById('org-form-title').textContent = type === 'project' ? '创建项目' : '创建成本中心';
      form.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function resetOrgForm() {
      state.editingOrgId = 0;
      const form = document.getElementById('org-form');
      form.reset();
      form.elements.parent_id.disabled = false;
      document.getElementById('org-form-title').textContent = '创建组织节点';
      document.getElementById('org-submit').textContent = '创建';
      document.getElementById('org-cancel-edit').hidden = true;
    }

    function beginPolicyEdit(id) {
      const row = (state.policies || []).find(item => item.id === id);
      if (!row) return;
      state.editingPolicyId = id;
      const form = document.getElementById('policy-form');
      ['name', 'default_group', 'monthly_budget_quota', 'daily_budget_quota', 'key_default_quota', 'status']
        .forEach(name => setFormValue(form, name, row[name]));
      setFormValue(form, 'allowed_models', row.allowed_models_list || []);
      setFormValue(form, 'denied_models', row.denied_models_list || []);
      document.getElementById('policy-form-title').textContent = '编辑 Policy #' + id;
      document.getElementById('policy-submit').textContent = '保存修改';
      document.getElementById('policy-cancel-edit').hidden = false;
      form.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function resetPolicyForm() {
      state.editingPolicyId = 0;
      const form = document.getElementById('policy-form');
      form.reset();
      document.getElementById('policy-form-title').textContent = '创建 Policy';
      document.getElementById('policy-submit').textContent = '创建';
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
      document.getElementById('key-form-title').textContent = '编辑企业 Key #' + id;
      document.getElementById('key-submit').textContent = '保存并同步';
      document.getElementById('key-cancel-edit').hidden = false;
      form.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function resetKeyForm() {
      state.editingKeyId = 0;
      const form = document.getElementById('key-form');
      form.reset();
      document.getElementById('key-form-title').textContent = '创建企业 Key';
      document.getElementById('key-submit').textContent = '创建并同步';
      document.getElementById('key-cancel-edit').hidden = true;
    }

    function beginBudgetEdit(id) {
      const row = (state.budgets || []).find(item => item.id === id);
      if (!row || row.source_type === 'policy') return;
      state.editingBudgetId = id;
      const form = document.getElementById('budget-form');
      ['scope_type', 'scope_id', 'budget_quota', 'status'].forEach(name => setFormValue(form, name, row[name]));
      updateBudgetScopeOptions();
      setFormValue(form, 'scope_id', row.scope_id);
      setFormValue(form, 'period_start', unixSecondsToLocalInput(row.period_start));
      setFormValue(form, 'period_end', unixSecondsToLocalInput(row.period_end));
      document.getElementById('budget-form-title').textContent = '编辑手工预算 #' + id;
      document.getElementById('budget-submit').textContent = '保存修改';
      document.getElementById('budget-cancel-edit').hidden = false;
      form.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function resetBudgetForm() {
      state.editingBudgetId = 0;
      const form = document.getElementById('budget-form');
      form.reset();
      updateBudgetScopeOptions();
      document.getElementById('budget-form-title').textContent = '创建手工预算';
      document.getElementById('budget-submit').textContent = '创建';
      document.getElementById('budget-cancel-edit').hidden = true;
    }

    async function deleteOrg(id) {
      const row = (state.orgs || []).find(item => item.id === id);
      if (!window.confirm('确认删除组织“' + (row ? row.name : '#' + id) + '”？被引用的组织不会被删除。')) return;
      try {
        await api('org-units/' + id, { method: 'DELETE' });
        if (state.editingOrgId === id) resetOrgForm();
        await Promise.all([loadReference(), loadOrgs()]);
      } catch (error) { showError(error); }
    }

    async function deletePolicy(id) {
      const row = (state.policies || []).find(item => item.id === id);
      if (!window.confirm('确认删除 Policy“' + (row ? row.name : '#' + id) + '”？被引用的 Policy 不会被删除。')) return;
      try {
        await api('policies/' + id, { method: 'DELETE' });
        if (state.editingPolicyId === id) resetPolicyForm();
        await Promise.all([loadReference(), loadPolicies(), loadOrgs()]);
      } catch (error) { showError(error); }
    }

    async function deleteKey(id) {
      const row = (state.keys || []).find(item => item.id === id);
      const name = row ? row.name : '#' + id;
      if (!window.confirm('确认删除企业 Key“' + name + '”？对应的 new-api Token 会立即撤销，历史用量仍保留。')) return;
      try {
        await api('keys/' + id, { method: 'DELETE' });
        if (state.editingKeyId === id) resetKeyForm();
        await Promise.all([loadReference(), loadKeys(), loadUsage(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    async function resetBudget(id) {
      if (!window.confirm('确认把该预算的已用额度归零？由该预算造成的 Key 阻断会被释放。')) return;
      try {
        await api('budgets/' + id + '/reset', { method: 'POST', body: '{}' });
        await Promise.all([loadBudgets(), loadKeys(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    async function deleteBudget(id) {
      if (!window.confirm('确认删除该手工预算？由该预算造成的 Key 阻断会被释放。')) return;
      try {
        await api('budgets/' + id, { method: 'DELETE' });
        if (state.editingBudgetId === id) resetBudgetForm();
        await Promise.all([loadBudgets(), loadKeys(), loadAudit()]);
      } catch (error) { showError(error); }
    }

    function table(rows, columns, actions) {
      if (!rows || rows.length === 0) return '<div class="status">暂无数据</div>';
      const head = columns.map(col => '<th>' + col.label + '</th>').join('') + (actions ? '<th>操作</th>' : '');
      const body = rows.map(row => {
        const cells = columns.map(col => {
          const value = col.format ? col.format(row[col.key], row) : formatValue(row[col.key]);
          return '<td>' + escapeHTML(formatValue(value)) + '</td>';
        }).join('');
        const actionCell = actions ? '<td>' + actions(row) + '</td>' : '';
        return '<tr>' + cells + actionCell + '</tr>';
      }).join('');
      return '<table><thead><tr>' + head + '</tr></thead><tbody>' + body + '</tbody></table>';
    }

    function formatValue(value) {
      if (value === null || value === undefined) return '';
      if (Array.isArray(value)) return value.join(', ');
      if (typeof value === 'object') return JSON.stringify(value);
      return String(value);
    }

    function formatUnixSeconds(value) {
      const seconds = Number(value || 0);
      if (!seconds) return '不限';
      return new Date(seconds * 1000).toLocaleString();
    }

    function unixSecondsToLocalInput(value) {
      const seconds = Number(value || 0);
      if (!seconds) return '';
      const date = new Date(seconds * 1000);
      const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
      return local.toISOString().slice(0, 16);
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
      if (tab === 'scopes') loadOrgs();
      if (tab === 'audit') loadAudit();
      if (tab === 'tokenop') loadTokenOperation();
    }

    async function loadMe() {
      const me = await api('auth/me');
      state.me = me;
      document.getElementById('me').textContent = me.username + ' / ' + me.hub_role + ' / new-api role ' + me.role;
      renderJSON('me-json', me);
    }

    async function loadOrgs() {
      const rows = await api('org-units');
      state.orgs = rows;
      document.getElementById('org-table').innerHTML = table(rows, [
        { key: 'id', label: 'ID' }, { key: 'name', label: '名称' }, { key: 'type', label: '类型' },
        { key: 'path', label: '路径' }, { key: 'default_group', label: 'group' },
        { key: 'default_policy_id', label: 'Policy' }, { key: 'newapi_user_id', label: '服务账号' },
        { key: 'status', label: '状态' }
      ], row => '<div class="row-actions"><button class="secondary" onclick="beginOrgEdit(' + row.id + ')">编辑</button>' +
        '<button class="danger" onclick="deleteOrg(' + row.id + ')">删除</button></div>');
      renderOrgTypeTable('project-table', rows.filter(row => row.type === 'project'));
      renderOrgTypeTable('cost-center-table', rows.filter(row => row.type === 'cost_center'));
    }

    function renderOrgTypeTable(containerId, rows) {
      const container = document.getElementById(containerId);
      if (!container) return;
      container.innerHTML = table(rows, [
        { key: 'id', label: 'ID' }, { key: 'name', label: '名称' }, { key: 'code', label: '编码' },
        { key: 'path', label: '路径' }, { key: 'default_policy_id', label: 'Policy' },
        { key: 'default_group', label: 'group' }, { key: 'status', label: '状态' }
      ], row => '<div class="row-actions"><button class="secondary" onclick="beginOrgEdit(' + row.id + ')">编辑</button>' +
        '<button class="danger" onclick="deleteOrg(' + row.id + ')">删除</button></div>');
    }

    async function loadPolicies() {
      const rows = await api('policies');
      state.policies = rows;
      document.getElementById('policy-table').innerHTML = table(rows, [
        { key: 'id', label: 'ID' }, { key: 'name', label: '名称' }, { key: 'default_group', label: 'group' },
        { key: 'allowed_models_list', label: '允许模型' }, { key: 'denied_models_list', label: '禁止模型' },
        { key: 'monthly_budget_quota', label: '月预算' }, { key: 'key_default_quota', label: 'Key quota' },
        { key: 'status', label: '状态' }
      ], row => '<div class="row-actions"><button class="secondary" onclick="beginPolicyEdit(' + row.id + ')">编辑</button>' +
        '<button class="danger" onclick="deletePolicy(' + row.id + ')">删除</button></div>');
    }

    async function loadKeys() {
      const rows = await api('keys');
      state.keys = rows;
      document.getElementById('key-table').innerHTML = table(rows, [
        { key: 'id', label: 'ID' }, { key: 'name', label: '名称' }, { key: 'org_unit_id', label: '组织' },
        { key: 'policy_id', label: '直接 Policy' }, { key: 'effective_policy_ids', label: '有效 Policy' },
        { key: 'newapi_token_id', label: 'new-api token' }, { key: 'key_fingerprint', label: '指纹' },
        { key: 'status', label: '管理员状态' }, { key: 'effective_status', label: '生效状态' },
        { key: 'active_budget_blocks', label: '预算阻断' }, { key: 'sync_status', label: '同步' }
      ], row => '<div class="row-actions"><button class="secondary" onclick="beginKeyEdit(' + row.id + ')">编辑</button>' +
        '<button class="secondary" onclick="syncKey(' + row.id + ')">同步</button>' +
        '<button class="secondary" onclick="rotateKey(' + row.id + ')">轮换</button>' +
        '<button class="danger" onclick="setKeyStatus(' + row.id + ', \'disable\')">禁用</button>' +
        '<button class="secondary" onclick="setKeyStatus(' + row.id + ', \'enable\')">启用</button>' +
        '<button class="danger" onclick="deleteKey(' + row.id + ')">删除</button></div>');
    }

    async function loadBudgets() {
      const rows = await api('budgets');
      state.budgets = rows;
      document.getElementById('budget-table').innerHTML = table(rows, [
        { key: 'id', label: 'ID' }, { key: 'scope_type', label: '范围' }, { key: 'scope_id', label: '范围 ID' },
        { key: 'source_type', label: '来源' }, { key: 'source_id', label: 'Policy ID' },
        { key: 'period_kind', label: '周期' },
        { key: 'period_start', label: '开始时间', format: formatUnixSeconds },
        { key: 'period_end', label: '结束时间', format: formatUnixSeconds },
        { key: 'budget_quota', label: '预算' }, { key: 'used_quota', label: '已用' },
        { key: 'active_block_count', label: '阻断 Key' }, { key: 'status', label: '状态' }
      ], row => row.source_type === 'policy'
        ? '<span class="status">通过 Policy 管理</span>'
        : '<div class="row-actions"><button class="secondary" onclick="beginBudgetEdit(' + row.id + ')">编辑</button>' +
          '<button class="secondary" onclick="resetBudget(' + row.id + ')">归零</button>' +
          '<button class="danger" onclick="deleteBudget(' + row.id + ')">删除</button></div>');
    }

    async function loadUsage() {
      const groupBy = document.getElementById('usage-group').value;
      const summary = await api('usage/summary?group_by=' + encodeURIComponent(groupBy));
      const details = await api('usage/details?limit=50');
      document.getElementById('usage-summary').innerHTML = table(summary, [
        { key: 'key', label: '维度' }, { key: 'quota', label: 'quota' }, { key: 'amount', label: '金额' }, { key: 'count', label: '次数' }
      ]);
      document.getElementById('usage-details').innerHTML = table(details, [
        { key: 'id', label: 'ID' }, { key: 'newapi_log_id', label: 'log' }, { key: 'enterprise_key_id', label: 'Key' },
        { key: 'model_name', label: '模型' }, { key: 'channel_id', label: '渠道' }, { key: 'quota', label: 'quota' }
      ]);
    }

    async function loadAudit() {
      const audits = await api('audit-logs?limit=50');
      const jobs = await api('sync-jobs?limit=50');
      document.getElementById('audit-table').innerHTML = table(audits, [
        { key: 'id', label: 'ID' }, { key: 'admin_username', label: '管理员' }, { key: 'action', label: '动作' },
        { key: 'target_type', label: '对象' }, { key: 'target_id', label: '对象 ID' }, { key: 'created_at', label: '时间' }
      ]);
      document.getElementById('sync-table').innerHTML = table(jobs, [
        { key: 'id', label: 'ID' }, { key: 'entity_type', label: '实体' }, { key: 'entity_id', label: '实体 ID' },
        { key: 'operation', label: '操作' }, { key: 'status', label: '状态' }, { key: 'error_message', label: '错误' }
      ]);
    }

    async function loadAdmins() {
      const rows = await api('admin-bindings');
      document.getElementById('admin-table').innerHTML = table(rows, [
        { key: 'id', label: 'ID' }, { key: 'newapi_user_id', label: 'new-api 用户' }, { key: 'newapi_username', label: '用户名' },
        { key: 'hub_role', label: 'Hub 角色' }, { key: 'scope_org_unit_id', label: '组织范围' }, { key: 'status', label: '状态' }
      ]);
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
      if (!confirm('轮换后旧 Key 会失效，继续吗？')) return;
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
      box.innerHTML = '完整 Key 只展示一次：<br><code>' + escapeHTML(key) + '</code>';
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
    document.querySelector('#budget-form [name="scope_type"]').addEventListener('change', updateBudgetScopeOptions);
    document.getElementById('admin-user-select').addEventListener('change', updateAdminUsername);
    document.getElementById('org-form').addEventListener('submit', async event => {
      event.preventDefault();
      try {
        const id = state.editingOrgId;
        const current = id ? (state.orgs || []).find(item => item.id === id) : null;
        const payload = Object.assign({}, current || {}, formJSON(event.target));
        await api(id ? 'org-units/' + id : 'org-units', { method: id ? 'PUT' : 'POST', body: JSON.stringify(payload) });
        resetOrgForm();
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
    refreshAll();
  </script>
</body>
</html>
`
