import React, { useEffect, useMemo, useState } from 'react'
import { Block, fetchBlockHeader } from '../lib/api'

function PoolBadge({ name }: { name: string }) {
  const palette = [
    'bg-rose-500', 'bg-sky-500', 'bg-emerald-500', 'bg-amber-500', 'bg-fuchsia-500', 'bg-cyan-500', 'bg-lime-500', 'bg-indigo-500'
  ]
  const i = Math.abs(name.split('').reduce((a, c) => a + c.charCodeAt(0), 0)) % palette.length
  return <span className={`px-2 py-0.5 text-xs rounded-full ${palette[i]} text-white`}>{name}</span>
}

export default function BlocksTable({ blocks, since }: { blocks: Block[]; since?: number }) {
  const [hdr, setHdr] = useState<Record<number, { timestamp: number; reward: number; hash: string }>>({})

  useEffect(() => {
    let cancelled = false
    async function run() {
      const unknowns = blocks.filter(b => b.pool === 'Unknown' && (!b.timestamp || !b.reward))
      const uniqueHeights = Array.from(new Set(unknowns.map(b => b.height))).filter(h => !(h in hdr))
      if (uniqueHeights.length === 0) return
      const results = await Promise.allSettled(uniqueHeights.map(h => fetchBlockHeader(h)))
      if (cancelled) return
      const next: Record<number, { timestamp: number; reward: number; hash: string }> = {}
      results.forEach((r, i) => {
        if (r.status === 'fulfilled' && r.value && r.value.status === 'OK') {
          const h = uniqueHeights[i]
          next[h] = { timestamp: r.value.timestamp, reward: r.value.reward, hash: r.value.hash }
        }
      })
      if (Object.keys(next).length) setHdr(prev => ({ ...prev, ...next }))
    }
    run()
    return () => { cancelled = true }
  }, [blocks])
  const rows = useMemo(() => {
    if (!since) return blocks
    return blocks.filter(b => {
      const effTs = b.timestamp || hdr[b.height]?.timestamp || 0
      return effTs && effTs >= since
    })
  }, [blocks, hdr, since])
  return (
    <div className="overflow-auto">
      <table className="min-w-full text-sm">
        <thead>
          <tr className="text-slate-400">
            <th className="text-left p-2">Height</th>
            <th className="text-left p-2">Pool</th>
            <th className="text-left p-2">Miner</th>
            <th className="text-left p-2">Reward</th>
            <th className="text-left p-2">Time</th>
            <th className="text-left p-2">Valid</th>
          </tr>
        </thead>
        <tbody>
          {rows.map(b => {
            const effTs = b.timestamp || hdr[b.height]?.timestamp || 0
            const effRw = b.reward || hdr[b.height]?.reward || 0
            return (
            <tr key={b.height} className="border-t border-slate-800">
              <td className="p-2 font-mono"><a href={`https://monerohash.com/explorer/search?value=${b.height}`}>{b.height.toLocaleString()} </a></td>
              <td className="p-2"><PoolBadge name={b.pool} /></td>
              <td className="p-2 truncate max-w-[240px]" title={b.miner || ''}>{b.miner || '-'}</td>
              <td className="p-2">{effRw ? (effRw / 1e12).toFixed(4) + ' XMR' : '-'}</td>
              <td className="p-2">{effTs ? new Date(effTs * 1000).toLocaleString() : '-'}</td>
              <td className="p-2">{b.valid ? 'Yes' : 'No'}</td>
            </tr>
            )})}
        </tbody>
      </table>
    </div>
  )
}
