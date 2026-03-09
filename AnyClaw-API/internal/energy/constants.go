package energy

const (
	AdoptCost        = 100  // 领养宠物消耗
	DailyConsume     = 10   // 每日维持消耗
	TaskCost         = 1    // 无法获取 token 时的兜底消耗
	MinEnergyForTask = 5    // 低于此值无法对话
	ZeroDaysToDelete = 3    // 连续无电天数后永久消失
	InviteReward     = 50   // 邀请奖励（双方各得）
	NewUserEnergy    = 100  // 新用户初始电量
	TokensPerEnergy  = 1000 // 每 1000 token 消耗 1 电量（按实际消耗计费）
)
