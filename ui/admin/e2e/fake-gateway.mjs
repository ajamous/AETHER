// Stand-in for services/gateway during the OIDC end-to-end test.
//
// Answers the /v1/* reads the console makes and records the
// Authorization header of each request, so the test can check that the
// console forwards the session's id_token as a Bearer credential
// (lib/api.ts). GET /__seen returns the recorded headers.

import { createServer } from 'node:http';

const port = Number(process.env.PORT || 8080);
const seen = [];

const routes = {
  '/v1/health': { ready: true, upstream: {} },
  '/v1/templates': { templates: [] },
  '/v1/smds/events': { length: 0, events: [] },
  '/v1/eim/devices': { length: 0, devices: [] },
};

createServer((req, res) => {
  const path = (req.url || '/').split('?')[0];
  if (path === '/__seen') {
    res.writeHead(200, { 'content-type': 'application/json' });
    return res.end(JSON.stringify(seen));
  }
  seen.push({ path, authorization: req.headers.authorization ?? null });
  const body = routes[path];
  if (!body) {
    res.writeHead(404);
    return res.end();
  }
  res.writeHead(200, { 'content-type': 'application/json' });
  res.end(JSON.stringify(body));
}).listen(port, () => console.log(`fake gateway on :${port}`));
