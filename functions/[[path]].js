const GITHUB_RAW = 'https://raw.githubusercontent.com/babywbx/BiliCDN/data';
const GITHUB_API = 'https://api.github.com/repos/babywbx/BiliCDN/branches/data';

// Cache: browser 5min, CF edge 10min, serve stale up to 1h while revalidating
const DATA_CACHE = 'public, max-age=300, s-maxage=600, stale-while-revalidate=3600';
const API_CACHE = 'public, max-age=300, s-maxage=600, stale-while-revalidate=3600';

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
    const cached = await cache.match(cacheKey);
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
        'Cache-Control': API_CACHE,
        'Access-Control-Allow-Origin': '*',
      },
    });
    context.waitUntil(cache.put(cacheKey, result.clone()));
    return result;
  }

  // Data files: proxy from GitHub with CF edge caching
  const contentType = ALLOWED_FILES[file];
  if (!contentType) {
    return context.next();
  }

  const cache = caches.default;
  const cacheKey = new Request(url.toString());
  const cached = await cache.match(cacheKey);
  if (cached) return cached;

  const ghResp = await fetch(GITHUB_RAW + '/' + file, {
    headers: { 'User-Agent': 'BiliCDN-Proxy' },
  });

  if (!ghResp.ok) {
    return new Response('Not Found', { status: 404 });
  }

  const body = await ghResp.arrayBuffer();
  const response = new Response(body, {
    headers: {
      'Content-Type': contentType,
      'Cache-Control': DATA_CACHE,
      'Access-Control-Allow-Origin': '*',
    },
  });

  context.waitUntil(cache.put(cacheKey, response.clone()));
  return response;
}
