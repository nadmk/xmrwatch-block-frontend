import React, { useEffect, useMemo, useState } from 'react'
import Card from '../components/Card'
import OwnershipPie from '../components/OwnershipPie'
import BlocksTable from '../components/BlocksTable'
import OwnershipOverTime from '../components/OwnershipOverTime'
import { Block, Ownership, fetchBlocks, fetchOwnership } from '../lib/api'

export default function Dashboard() {
  const [period, setPeriod] = useState<'24h' | 'lastN'>('24h')
  const [lastN, setLastN] = useState(1000)
  const [ownership, setOwnership] = useState<Ownership[] | null>(null)
  const [blocks, setBlocks] = useState<Block[]>([])
  const [loading, setLoading] = useState(true)

  const since = useMemo(() => {
    const now = Math.floor(Date.now() / 1000)
    switch (period) {
      case '24h': return now - 24 * 3600
      case 'lastN': return 0
    }
  }, [period])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    Promise.all([
      fetchOwnership(period === 'lastN' ? { lastN } : { since }),
      fetchBlocks({ limit: 300, since }),
    ]).then(([own, blks]) => {
      if (cancelled) return
      setOwnership(own)
      setBlocks(blks)
    }).finally(() => setLoading(false))
    const t = setInterval(() => {
      Promise.all([
        fetchOwnership(period === 'lastN' ? { lastN } : { since }),
        fetchBlocks({ limit: 300, since }),
      ]).then(([own, blks]) => {
        if (cancelled) return
        setOwnership(own)
        setBlocks(blks)
      })
    }, 30000)
    return () => { cancelled = true; clearInterval(t) }
  }, [period, lastN, since])

  return (
    <div className="max-w-7xl mx-auto p-4 space-y-4">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Monero Block Ownership</h1>
  <div className="flex items-center gap-3">
          <select className="bg-slate-900 border border-slate-700 rounded px-2 py-1" value={period} onChange={e => setPeriod(e.target.value as '24h' | 'lastN')}>
            <option value="24h">24 hours</option>
            <option value="lastN">Last N blocks</option>
          </select>
          {period === 'lastN' && (
            <input type="number" className="w-28 bg-slate-900 border border-slate-700 rounded px-2 py-1" value={lastN} min={100} max={100000} onChange={e => setLastN(parseInt(e.target.value) || 0)} />
          )}
        </div>
      </header>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card>
          <h2 className="text-lg mb-2">Ownership share</h2>
          {ownership ? <OwnershipPie data={ownership} /> : <div className="text-slate-400">Loading…</div>}
        </Card>
        <Card>
          <h2 className="text-lg mb-2">Ownership over time</h2>
          <OwnershipOverTime blocks={blocks} since={since} />
        </Card>
      </div>
      <Card>
        <h2 className="text-lg mb-2">Recent blocks ({blocks.length})</h2>
        <BlocksTable blocks={blocks} since={since} />
      </Card>
      {loading && <div className="text-slate-400">Refreshing…</div>}
      <footer className="pt-6 mt-6 border-t border-slate-800 text-sm text-slate-400">
        © Monero Watch is a ongoing project. If you'd like to support it you can donate XMR here:
        <span className="block mt-1 font-mono break-all">
          41xLnncmwXoDiaXyHMQqta81EQBo2DCBqFJRpQq7nRFRWp6SskWePa6GAkc1k8ZPKWUK9eLhCjHS6M28cUHu6y3KMHRAQtf
        </span>
      </footer>
    </div>
  )
}
