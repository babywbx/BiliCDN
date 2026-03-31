const GITHUB_RAW = 'https://raw.githubusercontent.com/babywbx/BiliCDN/data';
const GITHUB_API = 'https://api.github.com/repos/babywbx/BiliCDN/branches/data';

const ALLOWED_FILES = {
  'domains.txt': 'text/plain; charset=utf-8',
  'nodes.json': 'application/json; charset=utf-8',
  'nodes.yml': 'text/yaml; charset=utf-8',
  'nodes.txt': 'text/plain; charset=utf-8',
  'nodes.md': 'text/markdown; charset=utf-8',
};

export async function onRequest(context) {
  const url = new URL(context.request.url);
  const file = url.pathname.replace(/^\/+/, '');

  // API: return last commit date for the data branch
  if (file === 'api/updated') {
    const cache = caches.default;
    const cacheKey = new Request(url.toString());
    let cached = await cache.match(cacheKey);
    if (cached) return cached;

    const resp = await fetch(GITHUB_API, {
      headers: { 'User-Agent': 'BiliCDN-Proxy', 'Accept': 'application/json' },
    });
    if (!resp.ok) return new Response('{}', { status: 502 });

    const data = await resp.json();
    const date = data.commit?.commit?.committer?.date || null;
    const result = new Response(JSON.stringify({ date }), {
      headers: {
        'Content-Type': 'application/json',
        'Cache-Control': 'public, max-age=3600, s-maxage=3600',
        'Access-Control-Allow-Origin': '*',
      },
    });
    context.waitUntil(cache.put(cacheKey, result.clone()));
    return result;
  }

  const contentType = ALLOWED_FILES[file];
  if (!contentType) {
    return context.next();
  }

  const cacheKey = new Request(url.toString(), context.request);
  const cache = caches.default;

  // Try cache first
  let response = await cache.match(cacheKey);
  if (response) {
    return response;
  }

  // Fetch from GitHub
  const ghResp = await fetch(GITHUB_RAW + '/' + file, {
    headers: { 'User-Agent': 'BiliCDN-Proxy' },
  });

  if (!ghResp.ok) {
    return new Response('Not Found', { status: 404 });
  }

  const body = await ghResp.arrayBuffer();

  response = new Response(body, {
    headers: {
      'Content-Type': contentType,
      'Cache-Control': 'public, max-age=3600, s-maxage=3600',
      'Access-Control-Allow-Origin': '*',
      'X-Source': 'github-proxy',
    },
  });

  // Store in CF cache
  context.waitUntil(cache.put(cacheKey, response.clone()));

  return response;
}
