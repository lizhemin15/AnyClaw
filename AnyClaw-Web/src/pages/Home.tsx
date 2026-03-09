import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { getInstances, createInstance, type Instance } from '../api'

export default function Home() {
  const [instances, setInstances] = useState<Instance[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');

  const navigate = useNavigate();

  const loadInstances = () => {
    setLoading(true);
    getInstances()
      .then(setInstances)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load'))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    loadInstances();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (creating) return;
    setCreating(true);
    setError('');
    try {
      const inst = await createInstance(newName.trim() || 'instance');
      setInstances((prev) => [inst, ...prev]);
      setNewName('');
      navigate(`/instances/${inst.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create');
    } finally {
      setCreating(false);
    }
  };

  const formatDate = (s: string) => {
    try {
      return new Date(s).toLocaleString();
    } catch {
      return s;
    }
  };

  return (
    <div className="max-w-2xl mx-auto">
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-xl font-semibold text-slate-800">对话</h1>
      </div>

      <form onSubmit={handleCreate} className="flex flex-col sm:flex-row gap-3 mb-6">
        <input
          type="text"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          placeholder="新建对话名称"
          className="flex-1 px-4 py-3 border border-slate-300 rounded-xl focus:outline-none focus:ring-2 focus:ring-slate-400 focus:border-transparent"
        />
        <button
          type="submit"
          disabled={creating}
          className="px-6 py-3 bg-slate-800 text-white rounded-xl active:bg-slate-700 disabled:opacity-50 touch-target min-h-[48px]"
        >
          {creating ? '创建中...' : '新建'}
        </button>
      </form>

      {error && (
        <p className="mb-4 text-sm text-red-600 bg-red-50 p-3 rounded-lg">{error}</p>
      )}

      {loading ? (
        <p className="text-slate-500 py-8">加载中...</p>
      ) : instances.length === 0 ? (
        <p className="text-slate-500 py-8 text-center">暂无对话，点击上方新建</p>
      ) : (
        <ul className="space-y-3">
          {instances.map((inst) => (
            <li
              key={inst.id}
              onClick={() => navigate(`/instances/${inst.id}`)}
              className="bg-white border border-slate-200 rounded-xl p-4 active:bg-slate-50 transition-colors cursor-pointer touch-target min-h-[72px] flex items-center"
            >
              <div className="flex-1 min-w-0">
                <p className="font-medium text-slate-800 truncate">{inst.name}</p>
                <p className="text-sm text-slate-500 mt-0.5">
                  {formatDate(inst.created_at)}
                </p>
              </div>
              <span
                className={`flex-shrink-0 ml-3 px-2.5 py-1 text-xs rounded-full ${
                  inst.status === 'running'
                    ? 'bg-green-100 text-green-800'
                    : inst.status === 'creating'
                    ? 'bg-amber-100 text-amber-800'
                    : inst.status === 'error'
                    ? 'bg-red-100 text-red-800'
                    : 'bg-slate-100 text-slate-700'
                }`}
              >
                {inst.status === 'running' ? '运行中' : inst.status === 'creating' ? '创建中' : inst.status === 'error' ? '异常' : inst.status}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
