/**
 * Cloudflare Worker 静态资源入口
 *
 * 本 Worker 仅用于托管 public/ 下的静态资源。
 * 所有请求由 Static Assets (ASSETS binding) 处理。
 * 此文件作为兜底入口，确保 Worker 有可执行代码。
 */

export default {
  async fetch(request: Request, env: { ASSETS: { fetch: (req: Request) => Promise<Response> } }): Promise<Response> {
    const url = new URL(request.url);

    // 根路径重定向到 index.html
    if (url.pathname === "/" || url.pathname === "") {
      return Response.redirect(`${url.origin}/index.html`, 302);
    }

    // 其余请求交给 Static Assets 处理
    return env.ASSETS.fetch(request);
  },
};