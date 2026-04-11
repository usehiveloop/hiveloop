// Liveness endpoint for Railway health checks. Railway pings this path
// before promoting a new deployment to production; if it doesn't return
// 200, the new build stays in the failed state and the running deploy
// keeps serving traffic.
//
// Intentionally minimal — no DB, no external calls. Any 200 means the
// Next.js server is accepting connections, which is all we need to
// decide "don't cut over to a broken build".

export const dynamic = 'force-static'

export function GET() {
  return new Response('ok', {
    status: 200,
    headers: { 'content-type': 'text/plain; charset=utf-8' },
  })
}
