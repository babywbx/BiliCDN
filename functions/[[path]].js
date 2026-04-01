const GITHUB_RAW = 'https://raw.githubusercontent.com/babywbx/BiliCDN/data';
const GITHUB_API = 'https://api.github.com/repos/babywbx/BiliCDN/branches/data';

// Cache: browser 10min, CF edge 6h, serve stale up to 1 day while revalidating
// Branch metadata stays short-lived so every POP can switch to the new data revision
// without relying on local-only cache.delete() invalidation.
const DATA_CACHE = 'public, max-age=600, s-maxage=21600, stale-while-revalidate=86400';
const API_CACHE = 'public, max-age=600, s-maxage=300, stale-while-revalidate=600';
const BRANCH_CACHE = 'public, max-age=60, s-maxage=300, stale-while-revalidate=600';

const ALLOWED_FILES = {
  'domains.txt': 'text/plain; charset=utf-8',
  'nodes.json': 'application/json; charset=utf-8',
  'nodes.yml': 'text/yaml; charset=utf-8',
  'nodes.txt': 'text/plain; charset=utf-8',
  'nodes.md': 'text/markdown; charset=utf-8',
};

function dataCacheKey(origin, file, version) {
  return new Request(origin + '/' + file + '?__rev=' + encodeURIComponent(version));
}

async function fetchGitHubFile(file, version, forceFresh = false) {
  const rawUrl = 'https://raw.githubusercontent.com/babywbx/BiliCDN/' + version + '/' + file;
  const branchUrl = GITHUB_RAW + '/' + file + (forceFresh ? '?t=' + Date.now() : '');

  let ghResp = await fetch(rawUrl, {
    headers: { 'User-Agent': 'BiliCDN-Proxy' },
    cache: forceFresh ? 'no-store' : undefined,
  });
  if (ghResp.status === 404 && version !== 'data') {
    ghResp = await fetch(branchUrl, {
      headers: { 'User-Agent': 'BiliCDN-Proxy' },
      cache: forceFresh ? 'no-store' : undefined,
    });
  }

  if (!ghResp.ok) return null;

  return ghResp.arrayBuffer();
}

function makeDataResponse(body, contentType, version) {
  return new Response(body, {
    headers: {
      'Content-Type': contentType,
      'Cache-Control': DATA_CACHE,
      'Access-Control-Allow-Origin': '*',
      'X-BiliCDN-Data-Sha': version,
    },
  });
}

async function getBranchInfo(context, forceFresh = false) {
  const url = new URL(context.request.url);
  const cache = caches.default;
  const cacheKey = new Request(url.origin + '/__branch_info');

  if (forceFresh) {
    await cache.delete(cacheKey);
  } else {
    const cached = await cache.match(cacheKey);
    if (cached) {
      return cached.json();
    }
  }

  const apiUrl = GITHUB_API + (forceFresh ? '?t=' + Date.now() : '');
  const resp = await fetch(apiUrl, {
    headers: { 'User-Agent': 'BiliCDN-Proxy', 'Accept': 'application/json' },
    cache: forceFresh ? 'no-store' : undefined,
  });
  if (!resp.ok) return null;

  const data = await resp.json();
  const info = {
    sha: data.commit?.sha || null,
    date: data.commit?.commit?.committer?.date || null,
  };

  const result = new Response(JSON.stringify(info), {
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': BRANCH_CACHE,
    },
  });
  context.waitUntil(cache.put(cacheKey, result.clone()));
  return info;
}

export async function onRequest(context) {
  const url = new URL(context.request.url);
  const file = url.pathname.replace(/^\/+/, '');

  if (file === 'api/refresh') {
    if (context.request.method !== 'POST') {
      return new Response('Method Not Allowed', {
        status: 405,
        headers: { Allow: 'POST' },
      });
    }

    const secret = context.env?.REFRESH_TOKEN;
    const auth = context.request.headers.get('authorization');
    if (!secret) {
      return new Response(JSON.stringify({ error: 'refresh token not configured' }), {
        status: 503,
        headers: {
          'Content-Type': 'application/json',
          'Cache-Control': 'no-store',
        },
      });
    }
    if (auth !== 'Bearer ' + secret) {
      return new Response(JSON.stringify({ error: 'unauthorized' }), {
        status: 401,
        headers: {
          'Content-Type': 'application/json',
          'Cache-Control': 'no-store',
        },
      });
    }

    const branchInfo = await getBranchInfo(context, true);
    if (!branchInfo?.sha) {
      return new Response(JSON.stringify({ error: 'failed to refresh branch info' }), {
        status: 502,
        headers: {
          'Content-Type': 'application/json',
          'Cache-Control': 'no-store',
        },
      });
    }

    const cache = caches.default;
    const refreshed = [];
    const failed = [];

    for (const [name, contentType] of Object.entries(ALLOWED_FILES)) {
      const body = await fetchGitHubFile(name, branchInfo.sha, true);
      if (!body) {
        failed.push(name);
        continue;
      }

      const response = makeDataResponse(body, contentType, branchInfo.sha);
      const cacheKey = dataCacheKey(url.origin, name, branchInfo.sha);
      await cache.delete(cacheKey);
      context.waitUntil(cache.put(cacheKey, response.clone()));
      refreshed.push(name);
    }

    return new Response(JSON.stringify({
      ok: failed.length === 0,
      sha: branchInfo.sha,
      date: branchInfo.date,
      refreshedAt: new Date().toISOString(),
      refreshed,
      failed,
    }), {
      status: failed.length === 0 ? 200 : 207,
      headers: {
        'Content-Type': 'application/json',
        'Cache-Control': 'no-store',
      },
    });
  }

  // API: return last commit date for the data branch.
  // Reuse the same branch metadata as data files so timestamp and JSON stay in sync.
  if (file === 'api/updated') {
    const info = await getBranchInfo(context);
    if (!info?.date) return new Response('{}', { status: 502 });

    return new Response(JSON.stringify({ date: info.date, sha: info.sha }), {
      headers: {
        'Content-Type': 'application/json',
        'Cache-Control': API_CACHE,
        'Access-Control-Allow-Origin': '*',
        'X-BiliCDN-Data-Sha': info.sha || '',
      },
    });
  }

  // Data files: proxy from GitHub with CF edge caching
  const contentType = ALLOWED_FILES[file];
  if (!contentType) {
    return context.next();
  }

  const cache = caches.default;
  const branchInfo = await getBranchInfo(context);
  const version = branchInfo?.sha || 'data';
  const cacheKey = dataCacheKey(url.origin, file, version);

  const cached = await cache.match(cacheKey);
  if (cached) return cached;

  const body = await fetchGitHubFile(file, version);
  if (!body) {
    return new Response('Not Found', { status: 404 });
  }

  const response = makeDataResponse(body, contentType, version);

  context.waitUntil(cache.put(cacheKey, response.clone()));
  return response;
}
