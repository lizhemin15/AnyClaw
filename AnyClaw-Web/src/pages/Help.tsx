import { Link } from 'react-router-dom'

export default function Help() {
  return (
    <div className="max-w-2xl mx-auto">
      <h1 className="text-xl font-semibold text-slate-800 mb-6">使用帮助</h1>

      <div className="space-y-8">
        {/* 飞书绑定 */}
        <section className="bg-white rounded-xl border border-slate-200 p-4 sm:p-6">
          <h2 className="text-lg font-medium text-slate-800 mb-3 flex items-center gap-2">
            <span>📋</span> 如何通过网页端绑定飞书
          </h2>
          <p className="text-sm text-slate-600 mb-4">
            在网页端与你的小龙虾（宠物）聊天时，可直接发送消息完成飞书绑定，无需手动编辑配置文件。
          </p>
          <div className="space-y-4 text-sm">
            <div>
              <h3 className="font-medium text-slate-700 mb-2">第一步：在飞书开放平台创建应用</h3>
              <ol className="list-decimal list-inside space-y-1 text-slate-600">
                <li>打开 <a href="https://open.feishu.cn/app" target="_blank" rel="noopener noreferrer" className="text-indigo-600 underline">飞书开放平台</a> 或 <a href="https://open.feishu.cn/" target="_blank" rel="noopener noreferrer" className="text-indigo-600 underline">开发者后台</a></li>
                <li>登录飞书账号（网页登录即可，无需扫码）</li>
                <li>点击「创建企业自建应用」，填写应用名称</li>
                <li>创建完成后，在「凭证与基础信息」中获取 <strong>App ID</strong>（以 <code className="bg-slate-100 px-1 rounded">cli_</code> 开头）和 <strong>App Secret</strong></li>
              </ol>
            </div>
            <div>
              <h3 className="font-medium text-slate-700 mb-2">第二步：配置飞书应用权限</h3>
              <ol className="list-decimal list-inside space-y-1 text-slate-600">
                <li>在应用后台进入「权限管理」</li>
                <li>开通以下权限（搜索并添加）：
                  <ul className="list-disc list-inside ml-4 mt-1 space-y-0.5">
                    <li><code className="bg-slate-100 px-1 rounded text-xs">im:message</code> — 获取与发送单聊、群组消息</li>
                    <li><code className="bg-slate-100 px-1 rounded text-xs">im:message:send_as_bot</code> — 以应用身份发消息</li>
                    <li><code className="bg-slate-100 px-1 rounded text-xs">im:message.group_at_msg</code> — 接收群聊中 @ 机器人的消息</li>
                  </ul>
                </li>
                <li>在「应用功能」→「机器人」中开启机器人能力</li>
                <li>将应用发布到企业（自建应用需企业管理员审核）</li>
              </ol>
            </div>
            <div>
              <h3 className="font-medium text-slate-700 mb-2">第三步：在网页端聊天中发送绑定消息</h3>
              <p className="text-slate-600 mb-2">进入你的宠物对话页，发送以下任一格式的消息：</p>
              <div className="bg-slate-50 rounded-lg p-3 text-slate-700 font-mono text-xs space-y-1">
                <p>绑定飞书，app_id 是 cli_你的AppID，app_secret 是 你的AppSecret</p>
                <p className="text-slate-500 mt-2">或：配置飞书：cli_xxx / 我的secret</p>
              </div>
              <p className="text-slate-500 mt-2 text-xs">AI 会自动解析并写入配置，约 3 秒后宠物重启，飞书通道即可使用。</p>
            </div>
            <div>
              <h3 className="font-medium text-slate-700 mb-2">第四步：在飞书中使用</h3>
              <p className="text-slate-600">在飞书中搜索你创建的应用，发送消息即可与你的小龙虾对话。</p>
            </div>
          </div>
        </section>

        {/* 领养与金币 */}
        <section className="bg-white rounded-xl border border-slate-200 p-4 sm:p-6">
          <h2 className="text-lg font-medium text-slate-800 mb-3 flex items-center gap-2">
            <span>🪙</span> 领养与金币
          </h2>
          <ul className="space-y-2 text-sm text-slate-600">
            <li>· <strong>领养宠物</strong>：需要消耗金币（默认 100），领养后宠物永久存在</li>
            <li>· <strong>对话消耗</strong>：按 token 计费，约每 1000 token 消耗 1 金币</li>
            <li>· <strong>包月</strong>：可为单只宠物包月，包月后 30 天内对话不消耗金币</li>
            <li>· <strong>充值</strong>：可通过充值页扫码或联系管理员充值</li>
          </ul>
        </section>

        {/* 常见问题 */}
        <section className="bg-white rounded-xl border border-slate-200 p-4 sm:p-6">
          <h2 className="text-lg font-medium text-slate-800 mb-3 flex items-center gap-2">
            <span>❓</span> 常见问题
          </h2>
          <dl className="space-y-3 text-sm">
            <div>
              <dt className="font-medium text-slate-700">宠物显示「创建中」很久？</dt>
              <dd className="text-slate-600 mt-0.5">首次创建约需 1–2 分钟，请耐心等待。若超过 5 分钟仍无变化，可联系管理员检查服务器状态。</dd>
            </div>
            <div>
              <dt className="font-medium text-slate-700">飞书绑定后收不到回复？</dt>
              <dd className="text-slate-600 mt-0.5">请确认已在飞书应用后台开通消息相关权限，并将应用发布到企业。自建应用需企业管理员审核通过。</dd>
            </div>
            <div>
              <dt className="font-medium text-slate-700">金币不足无法对话？</dt>
              <dd className="text-slate-600 mt-0.5">需至少 5 金币才能发起对话。可通过充值获取，或为宠物包月后 30 天内不消耗金币。</dd>
            </div>
          </dl>
        </section>
      </div>

      <div className="mt-8 pt-6 border-t border-slate-200 text-center">
        <Link to="/" className="text-sm text-indigo-600 hover:underline">返回首页</Link>
      </div>
    </div>
  )
}
