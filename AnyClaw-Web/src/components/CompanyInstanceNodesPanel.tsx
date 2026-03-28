import { useMemo } from 'react'
import type { Instance } from '../api'
import { layoutAgents } from './collabTopologyUtils'

export type CompanyInstanceNodesPanelProps = {
  instances: Instance[]
  selectedId: number | null
  onSelect: (inst: Instance) => void
  className?: string
}

/** 编排模式：将账号下已招募实例（员工）显示为节点，点击切换当前实例的协作拓扑编辑目标 */
export default function CompanyInstanceNodesPanel({
  instances,
  selectedId,
  onSelect,
  className = '',
}: CompanyInstanceNodesPanelProps) {
  const pos = useMemo(() => layoutAgents(instances.length), [instances.length])

  if (instances.length === 0) return null

  return (
    <div className={`rounded-2xl border border-violet-200/80 bg-gradient-to-b from-violet-50/40 to-white p-4 sm:p-5 space-y-3 ${className}`}>
      <h3 className="text-sm font-semibold text-slate-800">公司员工</h3>
      <div className="relative w-full aspect-square max-h-[min(50vh,20rem)] mx-auto select-none rounded-xl border border-slate-200/80 bg-slate-50/50">
        {instances.map((inst, i) => {
          const { x, y } = pos[i]
          const selected = selectedId === inst.id
          return (
            <div
              key={inst.id}
              className="absolute -translate-x-1/2 -translate-y-1/2 z-[1]"
              style={{ left: `${x}%`, top: `${y}%`, width: '30%', maxWidth: '140px', minHeight: '52px' }}
            >
              <button
                type="button"
                aria-pressed={selected}
                onClick={() => onSelect(inst)}
                className={`w-full min-h-[48px] rounded-xl border-2 flex flex-col items-center justify-center px-1 py-1.5 text-center transition-shadow ${
                  selected
                    ? 'border-violet-600 bg-violet-50 shadow-md z-10'
                    : 'border-slate-200 bg-white hover:border-violet-300 hover:bg-slate-50'
                }`}
                title={inst.name}
              >
                <span className="text-[11px] font-medium text-slate-800 line-clamp-2 leading-tight">{inst.name}</span>
                <span className="text-[10px] text-slate-400 truncate w-full mt-0.5">#{inst.id}</span>
              </button>
            </div>
          )
        })}
      </div>
    </div>
  )
}
