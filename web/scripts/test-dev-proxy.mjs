import assert from 'node:assert/strict'
import { createServer as createHTTPServer } from 'node:http'
import { createHash } from 'node:crypto'
import { createServer as createViteServer } from 'vite'
import { fileURLToPath } from 'node:url'

const upstream = createHTTPServer((req, res) => {
  res.setHeader('Content-Type', 'application/json')
  res.setHeader(
    'Set-Cookie',
    'preview_session=test-only; Domain=example.test; Path=/zanelin/; Secure; HttpOnly; SameSite=Lax',
  )
  res.end(JSON.stringify({ path: req.url, origin: req.headers.origin, host: req.headers.host }))
})
const sockets = new Set()
let upgraded
upstream.on('upgrade', (req, socket) => {
  sockets.add(socket)
  socket.on('close', () => sockets.delete(socket))
  socket.on('error', () => {}) // The reset scenario intentionally tears down this test socket.
  upgraded = { path: req.url, origin: req.headers.origin }
  const accept = createHash('sha1')
    .update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11')
    .digest('base64')
  socket.write(
    `HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`,
  )
  const snapshot = Buffer.from(JSON.stringify({ type: 'snapshot', metrics: [{ agent_id: 'proxy-test' }] }))
  socket.write(Buffer.concat([Buffer.from([0x81, snapshot.length]), snapshot]))
  socket.on('data', (frame) => {
    // Acknowledge the native client's closing handshake rather than dropping it.
    if ((frame[0] & 0x0f) === 8) socket.end(Buffer.from([0x88, 0]))
  })
})
await new Promise((resolve) => upstream.listen(0, '127.0.0.1', resolve))
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
  const response = await fetch(`http://127.0.0.1:${port}/api/v1/admin/session`, {
    headers: { Origin: 'http://localhost:5173' },
  })
  assert.deepEqual(await response.json(), {
    path: '/zanelin/api/v1/admin/session',
    origin,
    host: upstream.address().address + ':' + upstream.address().port,
  })
  const cookie = response.headers.get('set-cookie')
  assert.match(cookie, /Path=\/(?:;|$)/)
  assert.doesNotMatch(cookie, /Domain=|Path=\/zanelin/)
  for (const attribute of ['Secure', 'HttpOnly', 'SameSite=Lax']) assert.ok(cookie.includes(attribute))
  const stream = await openRealtime(port)
  await closeRealtime(stream)
  assert.deepEqual(upgraded, { path: '/zanelin/api/v1/dashboard/realtime', origin })
  const interrupted = await openRealtime(port)
  const closed = waitForClose(interrupted)
  ;[...sockets].at(-1).resetAndDestroy()
  await closed
  const recovered = await openRealtime(port)
  await closeRealtime(recovered)
  console.log(
    'dev proxy prefix, cookie security, WS origin/frames, graceful close and upstream reset recovery passed (local mock only)',
  )
} finally {
  if (previousTarget === undefined) delete process.env.VPSMONITOR_API_TARGET
  else process.env.VPSMONITOR_API_TARGET = previousTarget
  for (const socket of sockets) socket.destroy()
  await vite?.close()
  upstream.closeAllConnections()
  await new Promise((resolve) => upstream.close(resolve))
}

async function openRealtime(port) {
  const stream = new WebSocket(`ws://127.0.0.1:${port}/api/v1/dashboard/realtime`)
  try {
    const snapshot = await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('websocket snapshot timeout')), 5000)
      const done = (fn, value) => {
        clearTimeout(timer)
        fn(value)
      }
      stream.addEventListener('message', (event) => done(resolve, JSON.parse(event.data)), { once: true })
      stream.addEventListener('error', () => done(reject, new Error('websocket proxy connection failed')), {
        once: true,
      })
      stream.addEventListener('close', () => done(reject, new Error('websocket closed before its snapshot')), {
        once: true,
      })
    })
    assert.deepEqual(snapshot, { type: 'snapshot', metrics: [{ agent_id: 'proxy-test' }] })
    return stream
  } catch (error) {
    stream.close()
    throw error
  }
}

function waitForClose(stream) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('websocket close timeout')), 5000)
    stream.addEventListener(
      'close',
      () => {
        clearTimeout(timer)
        resolve()
      },
      { once: true },
    )
  })
}

async function closeRealtime(stream) {
  const closed = waitForClose(stream)
  stream.close(1000)
  await closed
}
