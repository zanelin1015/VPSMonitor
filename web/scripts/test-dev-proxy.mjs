import assert from 'node:assert/strict'
import { createServer as createHTTPServer, request } from 'node:http'
import { createHash } from 'node:crypto'
import { createServer as createViteServer } from 'vite'
import { fileURLToPath } from 'node:url'

const upstream = createHTTPServer((req, res) => {
  res.setHeader('Content-Type', 'application/json')
  res.setHeader('Set-Cookie', 'preview_session=test-only; Domain=example.test; Path=/zanelin/; Secure; HttpOnly; SameSite=Lax')
  res.end(JSON.stringify({ path: req.url, origin: req.headers.origin, host: req.headers.host }))
})
const sockets = new Set()
let upgraded
upstream.on('upgrade', (req, socket) => {
  sockets.add(socket)
  socket.on('close', () => sockets.delete(socket))
  upgraded = { path: req.url, origin: req.headers.origin }
  const accept = createHash('sha1').update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64')
  socket.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`)
})
await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve))
const origin = `http://127.0.0.1:${upstream.address().port}`
const previousTarget = process.env.VPSMONITOR_API_TARGET
process.env.VPSMONITOR_API_TARGET = `${origin}/zanelin`
let vite
try {
  vite = await createViteServer({
    root: fileURLToPath(new URL('../', import.meta.url)),
    configFile: fileURLToPath(new URL('../vite.config.ts', import.meta.url)),
    logLevel: 'silent',
    server: { host: '127.0.0.1', port: 0 },
  })
  await vite.listen()
  const port = vite.httpServer.address().port
  const response = await fetch(`http://127.0.0.1:${port}/api/v1/admin/session`, { headers: { Origin: 'http://localhost:5173' } })
  assert.deepEqual(await response.json(), { path: '/zanelin/api/v1/admin/session', origin, host: upstream.address().address + ':' + upstream.address().port })
  const cookie = response.headers.get('set-cookie')
  assert.match(cookie, /Path=\/(?:;|$)/)
  assert.doesNotMatch(cookie, /Domain=|Path=\/zanelin/)
  for (const attribute of ['Secure', 'HttpOnly', 'SameSite=Lax']) assert.ok(cookie.includes(attribute))
  await new Promise((resolve, reject) => {
    const req = request({ host: '127.0.0.1', port, path: '/api/v1/dashboard/realtime', headers: {
      Connection: 'Upgrade', Upgrade: 'websocket', 'Sec-WebSocket-Version': '13',
      'Sec-WebSocket-Key': 'dGhlIHNhbXBsZSBub25jZQ==', Origin: 'http://localhost:5173',
    } })
    req.setTimeout(5000, () => req.destroy(new Error('websocket upgrade timeout')))
    req.on('error', reject)
    req.on('response', res => { res.resume(); reject(new Error(`unexpected websocket response ${res.statusCode}`)) })
    req.on('upgrade', (res, socket) => { socket.destroy(); resolve() })
    req.end()
  })
  assert.deepEqual(upgraded, { path: '/zanelin/api/v1/dashboard/realtime', origin })
  console.log('dev proxy prefix, cookie scope/security attributes and websocket origin tests passed (local mock only)')
} finally {
  if (previousTarget === undefined) delete process.env.VPSMONITOR_API_TARGET
  else process.env.VPSMONITOR_API_TARGET = previousTarget
  for (const socket of sockets) socket.destroy()
  await vite?.close()
  upstream.closeAllConnections()
  await new Promise(resolve => upstream.close(resolve))
}
