import { useState, useEffect, useMemo } from 'react'
import { getMyUsage, type UsageLogEntry } from '../api'
import SearchInput from '../components/SearchInput'
import Pagination from '../components/Pagination'

const PAGE_SIZE = 20

export default function Usage() {
  const [items, setItems] = useState<UsageLogEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(PAGE_SIZE)

  useEffect(() => {
    setLoading(true)
    getMyUsage(500, 0)
      .then((r) => setItems(r.items || []))
      .catch((e) => setError(e instanceof Error ? e.message : '加载失败'))
      .finally(() => setLoading(false))
  }, [])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return items
    return items.filter((e) =>
      (e.instance_name || '').toLowerCase().includes(q) ||
      (e.model || '').toLowerCase().includes(q)
    )
  }, [items, search])

  const paginated = useMemo(() => {
    const start = (page - 1) * pageSize
    return filtered.slice(start, start + pageSize)
  }, [filtered, page, pageSize])

  const totalCoins = paginated.reduce((s, i) => s + (i.coins_cost || 0), 0)

  useEffect(() => {
    setPage(1)
  }, [search])

  return (
    <div className="max-w-2xl mx-auto">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4">
        <h1 className="text-lg font-semibold text-slate-800">消耗记录</h1>
        {items.length > 0 && (
          <SearchInput value={search} onChange={setSearch} placeholder="按宠物或模型搜索" className="sm:w-48" />
        )}
      </div>
      {error && <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-lg">{error}</p>}
      {loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : items.length === 0 ? (
        <div className="text-center py-12 bg-slate-50 rounded-xl">
          <p className="text-slate-500">暂无消耗记录</p>
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-12 bg-slate-50 rounded-xl">
          <p className="text-slate-500">未找到匹配「{search}」的记录</p>
        </div>
      ) : (
        <>
          <div className="mb-4 p-3 bg-amber-50 rounded-lg text-sm text-slate-700">
            本页合计消耗 <span className="font-medium text-amber-700">{totalCoins}</span> 金币
          </div>
          <div className="space-y-2">
            {paginated.map((e) => (
              <div
                key={e.id}
                className="flex items-center justify-between gap-4 p-3 bg-white border border-slate-200 rounded-lg"
              >
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-slate-800 truncate">
                    {e.instance_name || `宠物 #${e.instance_id}`}
                  </p>
                  <p className="text-xs text-slate-500">{e.created_at} · {e.model}</p>
                </div>
                <span className="text-sm font-medium text-amber-600 flex-shrink-0">-{e.coins_cost} 金币</span>
              </div>
            ))}
          </div>
          <Pagination
            page={page}
            pageSize={pageSize}
            total={filtered.length}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        </>
      )}
    </div>
  )
}
