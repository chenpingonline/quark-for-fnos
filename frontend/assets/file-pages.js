/* Combine upstream pages into one complete list; never silently truncate a directory. */
(function (root) {
  'use strict';
  async function collectPages(fetchPage, { firstPage, isCurrent = () => true, onProgress = () => {} } = {}) {
    const files = new Map();
    let page = 1, total = 0;
    while (isCurrent()) {
      const result = page === 1 && firstPage ? firstPage : await fetchPage(page);
      if (!isCurrent()) return null;
      const batch = result.files || [];
      const countBefore = files.size;
      total = Math.max(0, Number(result.total) || 0);
      for (const file of batch) files.set(file.id, file);
      onProgress(files.size, total);
      if (total > 0 && files.size >= total) return [...files.values()];
      if (!batch.length) {
        if (total > files.size) throw Error('目录内容发生变化，未能读取完整列表，请重新加载');
        return [...files.values()];
      }
      if (files.size === countBefore) throw Error('网盘重复返回同一批文件，未能读取完整列表，请重新加载');
      if (!total && batch.length < 100) return [...files.values()];
      page++;
    }
    return null;
  }
  const api = { collectPages };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.QuarkFilePages = api;
})(typeof window !== 'undefined' ? window : globalThis);
