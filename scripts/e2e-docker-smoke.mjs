/**
 * Docker 管理页面冒烟测试（零依赖 CDP）。
 *
 * 用法：node scripts/e2e-docker-smoke.mjs [baseUrl] [username] [password]
 * 环境变量：E2E_BROWSER（浏览器路径）
 *
 * 说明：验证码已改为进程内存储，无法外部读取；请用
 *   APP__AUTH__CAPTCHA_ENABLED=false
 * 启动服务后再运行本脚本。
 *
 * 覆盖：登录 → Docker 概览/环境/授权/容器/镜像/网络/数据卷/堆栈/编辑器/
 *       Swarm 集群/节点/服务/任务/密钥/配置 逐页渲染，收集控制台异常与 4xx/5xx。
 */
import { spawn } from 'node:child_process'
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const baseUrl = (process.argv[2] || 'http://127.0.0.1:8090').replace(/\/$/, '')
const username = process.argv[3] || 'admin'
const password = process.argv[4] || process.env.E2E_ADMIN_PASSWORD || ''
if (!password) {
  console.error('缺少管理员密码：请用第 4 个参数或 E2E_ADMIN_PASSWORD')
  process.exit(2)
}
const CDP_PORT = 9335
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const BROWSERS = [
  process.env.E2E_BROWSER,
  'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  'C:/Program Files/Microsoft/Edge/Application/msedge.exe',
  'C:/Program Files/Google/Chrome/Application/chrome.exe'
].filter(Boolean)

async function api(path, options = {}) {
  const res = await fetch(baseUrl + path, options)
  const body = await res.json().catch(() => ({}))
  if (!res.ok || body.code !== 0) throw new Error(`${path}: ${body.message || res.status}`)
  return body.data
}

async function loginByApi() {
  const cap = await api('/api/v1/auth/captcha')
  if (cap.enabled) {
    throw new Error('验证码已开启：请用 APP__AUTH__CAPTCHA_ENABLED=false 启动服务后再运行本脚本')
  }
  return api('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password })
  })
}

class CDP {
  constructor(ws) {
    this.ws = ws
    this.id = 0
    this.pending = new Map()
    this.consoleErrors = []
    this.httpErrors = []
    this.networkFailures = []
    this.requests = new Map()
    ws.addEventListener('message', (ev) => this.onMessage(JSON.parse(ev.data)))
  }
  onMessage(msg) {
    if (msg.id && this.pending.has(msg.id)) {
      const { resolve, reject } = this.pending.get(msg.id)
      this.pending.delete(msg.id)
      msg.error ? reject(new Error(JSON.stringify(msg.error))) : resolve(msg.result)
      return
    }
    if (msg.method === 'Runtime.consoleAPICalled' && msg.params.type === 'error') {
      this.consoleErrors.push(msg.params.args.map((a) => a.value ?? a.description).join(' '))
    }
    if (msg.method === 'Runtime.exceptionThrown') {
      const d = msg.params.exceptionDetails
      this.consoleErrors.push(`${d.text} ${d.exception?.description || ''}`)
    }
    if (msg.method === 'Network.requestWillBeSent') {
      this.requests.set(msg.params.requestId, msg.params.request.url)
    }
    if (msg.method === 'Network.responseReceived') {
      const r = msg.params.response
      if (r.status >= 400 && !r.url.includes('favicon')) this.httpErrors.push(`${r.status} ${r.url}`)
    }
    if (msg.method === 'Network.loadingFailed') {
      const url = this.requests.get(msg.params.requestId) || ''
      const error = msg.params.errorText || 'loadingFailed'
      // 页面切换/主动 abort 不算错误
      if (!error.includes('ERR_ABORTED') && !url.includes('favicon')) {
        this.networkFailures.push(`${error} ${url}`)
      }
    }
    if (msg.method === 'Log.entryAdded' && msg.params.entry.level === 'error') {
      this.consoleErrors.push(msg.params.entry.text)
    }
  }
  send(method, params = {}) {
    const id = ++this.id
    this.ws.send(JSON.stringify({ id, method, params }))
    return new Promise((resolve, reject) => this.pending.set(id, { resolve, reject }))
  }
  async eval(expression) {
    const r = await this.send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.text)
    return r.result?.value
  }
}

async function connect() {
  for (let i = 0; i < 60; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/list`)).json()
      const page = list.find((t) => t.type === 'page')
      if (page?.webSocketDebuggerUrl) {
        const ws = new WebSocket(page.webSocketDebuggerUrl)
        await new Promise((res, rej) => {
          ws.addEventListener('open', res, { once: true })
          ws.addEventListener('error', rej, { once: true })
        })
        return new CDP(ws)
      }
    } catch {
      /* wait */
    }
    await sleep(300)
  }
  throw new Error('无法连接浏览器调试端口')
}

const shotsDir = join(root, '.tmp', 'docker-smoke')
mkdirSync(shotsDir, { recursive: true })

const browserPath = BROWSERS.find((p) => p)
if (!browserPath) {
  console.error('未找到 Edge/Chrome，可用 E2E_BROWSER 指定')
  process.exit(2)
}
const profile = mkdtempSync(join(tmpdir(), 'tomato-docker-e2e-'))
const browser = spawn(
  browserPath,
  [
    '--headless=new',
    '--disable-gpu',
    '--no-first-run',
    '--no-default-browser-check',
    `--remote-debugging-port=${CDP_PORT}`,
    `--user-data-dir=${profile}`,
    'about:blank'
  ],
  { stdio: 'ignore' }
)

const results = []
const check = (name, ok, extra = '') => {
  results.push({ name, ok, extra })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${extra ? '  ' + extra : ''}`)
}

let capRow
let sliderRow
let token = ''

try {
  const tokenResp = await loginByApi()
  token = tokenResp.accessToken
  const authHeaders = { Authorization: `Bearer ${token}` }

  // 临时关闭验证码 + 滑块，让无头浏览器可以走真实登录表单（结束后恢复）。
  const configs = await api('/api/v1/system/configs', { headers: authHeaders })
  capRow = configs.find((row) => row.key === 'sys.login.captcha')
  sliderRow = configs.find((row) => row.key === 'sys.login.slider')
  const putCfg = async (row, value) => {
    if (!row?.id) return null
    const res = await fetch(`${baseUrl}/api/v1/system/configs/${row.id}`, {
      method: 'PUT',
      headers: { ...authHeaders, 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: row.name,
        key: row.key,
        value: String(value),
        valueType: row.valueType || 'bool',
        isPublic: row.isPublic ?? true,
        remark: row.remark || ''
      })
    })
    return res.json().catch(() => null)
  }
  await putCfg(capRow, false)
  await putCfg(sliderRow, false)
  // 运行期开关有 5 秒缓存；等缓存过期并确认验证码接口已关闭后再打开登录页。
  await sleep(6000)
  for (let i = 0; i < 10; i++) {
    const cap = await fetch(`${baseUrl}/api/v1/auth/captcha`).then((r) => r.json())
    if (cap?.data?.enabled === false) break
    await sleep(1000)
  }

  const cdp = await connect()
  await cdp.send('Page.enable')
  await cdp.send('Runtime.enable')
  await cdp.send('Network.enable')
  await cdp.send('Log.enable')

  await cdp.send('Page.navigate', { url: `${baseUrl}/#/auth/login` })
  await sleep(2500)
  const submitted = await cdp.eval(`(() => {
    const setVal = (el, v) => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
      setter.call(el, v)
      el.dispatchEvent(new Event('input', { bubbles: true }))
      el.dispatchEvent(new Event('change', { bubbles: true }))
    }
    const inputs = [...document.querySelectorAll('input')]
    const u = inputs.find((i) => (i.placeholder || '').includes('账号'))
    const p = inputs.find((i) => (i.placeholder || '').includes('密码'))
    if (!u || !p) return 'missing-input'
    setVal(u, ${JSON.stringify(username)})
    setVal(p, ${JSON.stringify(password)})
    const btn = [...document.querySelectorAll('button')].find((b) => (b.innerText || '').replace(/\\s+/g, '').includes('登录'))
    if (!btn) return 'no-button'
    btn.click()
    return 'submitted'
  })()`)
  check('登录页提交', submitted === 'submitted', submitted)
  let loggedIn = false
  for (let i = 0; i < 30; i++) {
    await sleep(500)
    const hash = (await cdp.eval('location.hash')) || ''
    if (!hash.includes('/auth/login') && hash.length > 2) {
      loggedIn = true
      break
    }
  }
  check('登录成功', loggedIn, await cdp.eval('location.hash'))

  const envs = await api('/api/v1/docker/environments', { headers: authHeaders })
  const envId = envs?.[0]?.id || ''
  const pages = [
    ['overview', '/docker/overview', 'Docker 概览'],
    ['environments', '/docker/environments', 'Docker 环境'],
    ['grants', '/docker/grants', '资源授权'],
    ['containers', '/docker/containers', '容器'],
    ['images', '/docker/images', '镜像'],
    ['networks', '/docker/networks', '网络'],
    ['volumes', '/docker/volumes', '数据卷'],
    ['stacks', '/docker/stacks', 'Stack'],
    ['stack-editor', `/docker/stacks/editor${envId ? `?envId=${envId}` : ''}`, '新建 Stack'],
    ['swarm-cluster', '/docker/swarm/cluster', 'Swarm'],
    ['swarm-nodes', '/docker/swarm/nodes', '节点'],
    ['swarm-services', '/docker/swarm/services', '服务'],
    ['swarm-tasks', '/docker/swarm/tasks', '任务'],
    ['swarm-secrets', '/docker/swarm/secrets', '密钥'],
    ['swarm-configs', '/docker/swarm/configs', '配置']
  ]

  for (const [name, path, expect] of pages) {
    cdp.consoleErrors.length = 0
    cdp.httpErrors.length = 0
    cdp.networkFailures.length = 0
    await cdp.send('Page.navigate', { url: `${baseUrl}/#${path}` })
    await sleep(2200)
    const text = await cdp.eval('document.body?.innerText || ""')
    const shot = await cdp.send('Page.captureScreenshot', { format: 'png' })
    writeFileSync(join(shotsDir, `${name}.png`), Buffer.from(shot.data, 'base64'))
    const notFound = text.includes('组件未找到') || text.includes('404') || text.includes('页面不存在')
    check(
      `页面 ${path}`,
      text.includes(expect) && !notFound,
      `${text.replace(/\s+/g, ' ').slice(0, 90)}`
    )
    if (cdp.consoleErrors.length) {
      check(`控制台错误 ${path}`, false, cdp.consoleErrors.slice(0, 2).join(' | '))
    }
    if (cdp.httpErrors.length) {
      check(`HTTP 错误 ${path}`, false, cdp.httpErrors.slice(0, 2).join(' | '))
    }
    if (cdp.networkFailures.length) {
      check(`网络请求失败 ${path}`, false, cdp.networkFailures.slice(0, 2).join(' | '))
    }
    cdp.consoleErrors.length = 0
    cdp.httpErrors.length = 0
    cdp.networkFailures.length = 0
  }
} catch (err) {
  check('测试执行', false, err instanceof Error ? err.message : String(err))
} finally {
  // 恢复登录开关，避免影响后续手工使用。
  // 浏览器登录/退出可能让旧 access token 失效，这里重新登录一次再恢复。
  try {
    const fresh = await loginByApi()
    const authHeaders = { Authorization: `Bearer ${fresh.accessToken}` }
    for (const [row, value] of [[capRow, true], [sliderRow, true]]) {
      if (!row?.id) continue
      await fetch(`${baseUrl}/api/v1/system/configs/${row.id}`, {
        method: 'PUT',
        headers: { ...authHeaders, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: row.name,
          key: row.key,
          value: String(value),
          valueType: row.valueType || 'bool',
          isPublic: row.isPublic ?? true,
          remark: row.remark || ''
        })
      })
    }
  } catch {
    /* ignore */
  }
  try {
    browser.kill()
  } catch {
    /* ignore */
  }
  await sleep(500)
  try {
    rmSync(profile, { recursive: true, force: true })
  } catch {
    /* ignore */
  }
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} passed，截图目录：${shotsDir}`)
if (failed.length) process.exit(1)