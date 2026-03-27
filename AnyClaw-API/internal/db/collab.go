package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
)

// InstanceAgentRow 实例内员工（与 config 中 agent id 对应）
type InstanceAgentRow struct {
	ID          int64  `json:"id"`
	InstanceID  int64  `json:"instance_id"`
	UserID      int64  `json:"user_id"`
	AgentSlug   string `json:"agent_slug"`
	DisplayName string `json:"display_name"`
}

// TopologyEdgeRow 无向边，存 canonical lo < hi
type TopologyEdgeRow struct {
	AgentSlugLo string `json:"agent_slug_lo"`
	AgentSlugHi string `json:"agent_slug_hi"`
}

// InternalMailRow 内部邮件
type InternalMailRow struct {
	ID               int64  `json:"id"`
	InstanceID       int64  `json:"instance_id"`
	ThreadID         string `json:"thread_id"`
	FromSlug         string `json:"from_slug"`
	ToSlug           string `json:"to_slug"`
	Subject          string `json:"subject"`
	Body             string `json:"body"`
	InReplyTo        *int64 `json:"in_reply_to,omitempty"`
	TopologyVersion  int64  `json:"topology_version"`
	CreatedAt        string `json:"created_at"`
}

func isMySQLError(err error, code uint16) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == code
}

const (
	// MaxInstanceAgentSlugRunes agent_slug 最大 Unicode 字符数（与表字段一致）
	MaxInstanceAgentSlugRunes = 128
	// MaxInstanceAgentDisplayRunes 展示名最大 Unicode 字符数（与表字段一致）
	MaxInstanceAgentDisplayRunes = 255
	// MaxCollaborationAgentsPerInstance 单实例协作员工上限
	MaxCollaborationAgentsPerInstance = 64
	// MaxCollaborationEdgesPerInstance 单实例无向连线上限
	MaxCollaborationEdgesPerInstance = 4096
)

// validateInstanceAgentRows 保存前校验：至少一人、slug/展示名非空、列表内无重复、长度与表字段一致。
func validateInstanceAgentRows(agents []InstanceAgentRow) error {
	if len(agents) == 0 {
		return fmt.Errorf("至少保留一名员工")
	}
	if len(agents) > MaxCollaborationAgentsPerInstance {
		return fmt.Errorf("单个实例最多配置 %d 名员工", MaxCollaborationAgentsPerInstance)
	}
	seenSlug := make(map[string]struct{})
	seenName := make(map[string]struct{})
	for _, a := range agents {
		slug := strings.TrimSpace(a.AgentSlug)
		name := strings.TrimSpace(a.DisplayName)
		if slug == "" || name == "" {
			return fmt.Errorf("agent_slug 与展示名均不能为空")
		}
		if utf8.RuneCountInString(slug) > MaxInstanceAgentSlugRunes {
			return fmt.Errorf("agent_slug %q 过长（最多 %d 字）", slug, MaxInstanceAgentSlugRunes)
		}
		if utf8.RuneCountInString(name) > MaxInstanceAgentDisplayRunes {
			return fmt.Errorf("展示名 %q 过长（最多 %d 字）", name, MaxInstanceAgentDisplayRunes)
		}
		if _, ok := seenSlug[slug]; ok {
			return fmt.Errorf("列表中存在重复的 agent_slug: %q", slug)
		}
		seenSlug[slug] = struct{}{}
		if _, ok := seenName[name]; ok {
			return fmt.Errorf("列表中存在重复的展示名: %q", name)
		}
		seenName[name] = struct{}{}
	}
	return nil
}

func friendlyInstanceAgentInsertErr(err error) error {
	var me *mysql.MySQLError
	if !errors.As(err, &me) || me.Number != 1062 {
		return err
	}
	msg := me.Message
	if strings.Contains(msg, "uk_user_display") {
		return fmt.Errorf("展示名在同一账号下须唯一（不能与其他实例或本列表冲突）")
	}
	if strings.Contains(msg, "uk_instance_slug") {
		return fmt.Errorf("同一实例内 agent_slug 须唯一")
	}
	return fmt.Errorf("员工数据违反唯一约束，请检查 agent_slug 与展示名")
}

// SeedDefaultCollabAgentsForNewInstance 新实例写入默认员工 main，与容器默认 agent id 对齐；展示名在 user 维度唯一冲突时带实例 id。
func (d *DB) SeedDefaultCollabAgentsForNewInstance(instanceID, userID int64, instanceName string) error {
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM instance_agents WHERE instance_id = ?`, instanceID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	base := strings.TrimSpace(instanceName)
	if base == "" {
		base = "员工"
	}
	display := base + "·主理"
	_, err := d.Exec(
		`INSERT INTO instance_agents (instance_id, user_id, agent_slug, display_name) VALUES (?, ?, 'main', ?)`,
		instanceID, userID, display,
	)
	if err == nil {
		_, _ = d.Exec(`UPDATE instances SET agent_topology_version = COALESCE(agent_topology_version, 0) + 1 WHERE id = ?`, instanceID)
		return nil
	}
	if isMySQLError(err, 1062) {
		fallback := fmt.Sprintf("主理·%d", instanceID)
		_, err2 := d.Exec(
			`INSERT INTO instance_agents (instance_id, user_id, agent_slug, display_name) VALUES (?, ?, 'main', ?)`,
			instanceID, userID, fallback,
		)
		if err2 == nil {
			_, _ = d.Exec(`UPDATE instances SET agent_topology_version = COALESCE(agent_topology_version, 0) + 1 WHERE id = ?`, instanceID)
		}
		return err2
	}
	return err
}

// BackfillCollabAgentsForInstancesWithoutRoster 为尚无协作员工的实例补默认 main（幂等）。
func (d *DB) BackfillCollabAgentsForInstancesWithoutRoster() (int, error) {
	rows, err := d.Query(`
		SELECT i.id, i.user_id, i.name FROM instances i
		WHERE NOT EXISTS (SELECT 1 FROM instance_agents ia WHERE ia.instance_id = i.id)`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, uid int64
		var name sql.NullString
		if err := rows.Scan(&id, &uid, &name); err != nil {
			return n, err
		}
		nm := ""
		if name.Valid {
			nm = name.String
		}
		if err := d.SeedDefaultCollabAgentsForNewInstance(id, uid, nm); err != nil {
			log.Printf("[db] backfill collab agents instance %d: %v", id, err)
			continue
		}
		n++
	}
	return n, rows.Err()
}

func canonicalEdge(a, b string) (lo, hi string) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a < b {
		return a, b
	}
	return b, a
}

// GetAgentTopologyVersion 当前拓扑版本（随员工/边变更递增）
func (d *DB) GetAgentTopologyVersion(instanceID int64) (int64, error) {
	var v int64
	err := d.QueryRow(`SELECT COALESCE(agent_topology_version, 0) FROM instances WHERE id = ?`, instanceID).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}

func (d *DB) bumpAgentTopologyVersion(tx *sql.Tx, instanceID int64) error {
	_, err := tx.Exec(`UPDATE instances SET agent_topology_version = COALESCE(agent_topology_version, 0) + 1 WHERE id = ?`, instanceID)
	return err
}

// ListInstanceAgents 某实例下的员工列表
func (d *DB) ListInstanceAgents(instanceID int64) ([]InstanceAgentRow, error) {
	rows, err := d.Query(
		`SELECT id, instance_id, user_id, agent_slug, display_name FROM instance_agents WHERE instance_id = ? ORDER BY agent_slug`,
		instanceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []InstanceAgentRow
	for rows.Next() {
		var r InstanceAgentRow
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.UserID, &r.AgentSlug, &r.DisplayName); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func displayNameAvailableForUserTx(tx *sql.Tx, userID int64, name string) (bool, error) {
	var n int
	err := tx.QueryRow(
		`SELECT COUNT(*) FROM instance_agents WHERE user_id = ? AND display_name = ?`,
		userID, name,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

func pickCollabDisplayNameForSyncTx(tx *sql.Tx, userID, instanceID int64, slug string) (string, error) {
	candidates := []string{
		slug,
		fmt.Sprintf("%s·%d", slug, instanceID),
	}
	for i := 2; i < 50; i++ {
		candidates = append(candidates, fmt.Sprintf("%s·%d·%d", slug, instanceID, i))
	}
	for _, c := range candidates {
		if c == "" || utf8.RuneCountInString(c) > MaxInstanceAgentDisplayRunes {
			continue
		}
		ok, err := displayNameAvailableForUserTx(tx, userID, c)
		if err != nil {
			return "", err
		}
		if ok {
			return c, nil
		}
	}
	return "", fmt.Errorf("无法为员工 slug %q 生成唯一展示名", slug)
}

// EnsureInstanceAgentSlugs 按容器 agents.list 补全尚未出现在协作表中的 agent_slug（仅追加，不删不改已有行）；展示名默认用 slug，账号内冲突时自动加后缀。
func (d *DB) EnsureInstanceAgentSlugs(instanceID, userID int64, rawSlugs []string) (added int, err error) {
	seen := make(map[string]struct{})
	var slugs []string
	for _, raw := range rawSlugs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if e := validateCollabAgentSlugSyntax(s); e != nil {
			return 0, e
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	if len(slugs) == 0 {
		return 0, nil
	}

	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var curCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM instance_agents WHERE instance_id = ?`, instanceID).Scan(&curCount); err != nil {
		return 0, err
	}

	added = 0
	for _, slug := range slugs {
		var exists int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM instance_agents WHERE instance_id = ? AND agent_slug = ?`,
			instanceID, slug,
		).Scan(&exists); err != nil {
			return 0, err
		}
		if exists > 0 {
			continue
		}
		if curCount >= MaxCollaborationAgentsPerInstance {
			return 0, fmt.Errorf("协作员工已达上限（%d 名），无法自动同步 slug %q", MaxCollaborationAgentsPerInstance, slug)
		}
		displayName, err := pickCollabDisplayNameForSyncTx(tx, userID, instanceID, slug)
		if err != nil {
			return 0, err
		}
		_, err = tx.Exec(
			`INSERT INTO instance_agents (instance_id, user_id, agent_slug, display_name) VALUES (?, ?, ?, ?)`,
			instanceID, userID, slug, displayName,
		)
		if err != nil {
			if fe := friendlyInstanceAgentInsertErr(err); fe != err {
				return 0, fe
			}
			return 0, fmt.Errorf("insert agent %q: %w", slug, err)
		}
		curCount++
		added++
	}

	if added > 0 {
		if err := d.bumpAgentTopologyVersion(tx, instanceID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return added, nil
}

// normalizeCollabSlugList 去重、排序，用于协作 roster 快照 JSON。
func normalizeCollabSlugList(raw []string) []string {
	seen := make(map[string]struct{})
	var slugs []string
	for _, s0 := range raw {
		s := strings.TrimSpace(s0)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	return slugs
}

// SetCollabRosterSlugsJSON 将容器或网页保存的 agent_slug 列表写入 instances.collab_roster_slugs（JSON 数组）。
func (d *DB) SetCollabRosterSlugsJSON(instanceID int64, rawSlugs []string) error {
	slugs := normalizeCollabSlugList(rawSlugs)
	if len(slugs) == 0 {
		_, err := d.Exec(`UPDATE instances SET collab_roster_slugs = NULL WHERE id = ?`, instanceID)
		return err
	}
	b, err := json.Marshal(slugs)
	if err != nil {
		return err
	}
	_, err = d.Exec(`UPDATE instances SET collab_roster_slugs = ? WHERE id = ?`, string(b), instanceID)
	return err
}

func stringSliceEqualSorted(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SyncCollabAgentsFromStoredSlugs 合并 collab_roster_slugs 快照、instance_agents 与拓扑边端点，再调用 EnsureInstanceAgentSlugs，供网页 GET 协作名单时自动补全节点（不依赖容器单独同步）。
func (d *DB) SyncCollabAgentsFromStoredSlugs(instanceID, userID int64) (added int, err error) {
	var raw sql.NullString
	if err := d.QueryRow(`SELECT collab_roster_slugs FROM instances WHERE id = ?`, instanceID).Scan(&raw); err != nil {
		return 0, err
	}
	var fromJSON []string
	if raw.Valid && strings.TrimSpace(raw.String) != "" {
		if err := json.Unmarshal([]byte(raw.String), &fromJSON); err != nil {
			fromJSON = nil
		}
	}
	jsonNorm := normalizeCollabSlugList(fromJSON)

	ia, err := d.ListInstanceAgents(instanceID)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]struct{})
	for _, s := range jsonNorm {
		seen[s] = struct{}{}
	}
	for _, a := range ia {
		s := strings.TrimSpace(a.AgentSlug)
		if s != "" {
			seen[s] = struct{}{}
		}
	}
	edges, err := d.ListTopologyEdges(instanceID)
	if err != nil {
		return 0, err
	}
	for _, e := range edges {
		if s := strings.TrimSpace(e.AgentSlugLo); s != "" {
			seen[s] = struct{}{}
		}
		if s := strings.TrimSpace(e.AgentSlugHi); s != "" {
			seen[s] = struct{}{}
		}
	}
	var union []string
	for s := range seen {
		union = append(union, s)
	}
	slugs := normalizeCollabSlugList(union)
	if len(slugs) == 0 {
		return 0, nil
	}
	if !stringSliceEqualSorted(slugs, jsonNorm) {
		if err := d.SetCollabRosterSlugsJSON(instanceID, slugs); err != nil {
			return 0, err
		}
	}
	return d.EnsureInstanceAgentSlugs(instanceID, userID, slugs)
}

// BackfillCollabRosterSlugsColumn 为已有 instance_agents 但尚未写入快照列的实例补一行 JSON（迁移用，幂等）。
func (d *DB) BackfillCollabRosterSlugsColumn() (int, error) {
	rows, err := d.Query(`
		SELECT i.id FROM instances i
		WHERE (i.collab_roster_slugs IS NULL OR TRIM(i.collab_roster_slugs) = '')
		  AND EXISTS (SELECT 1 FROM instance_agents ia WHERE ia.instance_id = i.id)`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, iid := range ids {
		list, err := d.ListInstanceAgents(iid)
		if err != nil {
			log.Printf("[db] backfill collab_roster_slugs instance %d: %v", iid, err)
			continue
		}
		var slugs []string
		for _, a := range list {
			s := strings.TrimSpace(a.AgentSlug)
			if s != "" {
				slugs = append(slugs, s)
			}
		}
		slugs = normalizeCollabSlugList(slugs)
		if len(slugs) == 0 {
			continue
		}
		if err := d.SetCollabRosterSlugsJSON(iid, slugs); err != nil {
			log.Printf("[db] backfill collab_roster_slugs instance %d: %v", iid, err)
			continue
		}
		n++
	}
	return n, nil
}

// ReplaceInstanceAgents 全量替换实例员工（事务）；展示名在 user_id 下唯一由 DB 保证
func (d *DB) ReplaceInstanceAgents(instanceID, userID int64, agents []InstanceAgentRow) error {
	if err := validateInstanceAgentRows(agents); err != nil {
		return err
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM instance_topology_edges WHERE instance_id = ?`, instanceID); err != nil {
		return fmt.Errorf("clear edges: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM instance_agents WHERE instance_id = ?`, instanceID); err != nil {
		return fmt.Errorf("clear agents: %w", err)
	}
	for _, a := range agents {
		slug := strings.TrimSpace(a.AgentSlug)
		name := strings.TrimSpace(a.DisplayName)
		_, err := tx.Exec(
			`INSERT INTO instance_agents (instance_id, user_id, agent_slug, display_name) VALUES (?, ?, ?, ?)`,
			instanceID, userID, slug, name,
		)
		if err != nil {
			if fe := friendlyInstanceAgentInsertErr(err); fe != err {
				return fe
			}
			return fmt.Errorf("insert agent %q: %w", slug, err)
		}
	}
	if err := d.bumpAgentTopologyVersion(tx, instanceID); err != nil {
		return err
	}
	return tx.Commit()
}

// ListTopologyEdges 列出无向边（返回规范化 lo/hi）
func (d *DB) ListTopologyEdges(instanceID int64) ([]TopologyEdgeRow, error) {
	rows, err := d.Query(
		`SELECT agent_slug_lo, agent_slug_hi FROM instance_topology_edges WHERE instance_id = ? ORDER BY agent_slug_lo, agent_slug_hi`,
		instanceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TopologyEdgeRow
	for rows.Next() {
		var r TopologyEdgeRow
		if err := rows.Scan(&r.AgentSlugLo, &r.AgentSlugHi); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// ReplaceTopologyEdges 全量替换边；slug 须已存在于 instance_agents
func (d *DB) ReplaceTopologyEdges(instanceID int64, pairs [][2]string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	slugSet := make(map[string]struct{})
	sr, err := d.ListInstanceAgents(instanceID)
	if err != nil {
		return err
	}
	for _, a := range sr {
		slugSet[a.AgentSlug] = struct{}{}
	}

	if len(slugSet) == 0 && len(pairs) > 0 {
		return fmt.Errorf("请先在「员工」页保存至少一名员工，再添加连线")
	}

	seen := make(map[string]struct{})
	var uniq [][2]string
	for _, p := range pairs {
		a, b := strings.TrimSpace(p[0]), strings.TrimSpace(p[1])
		if a == "" || b == "" {
			return fmt.Errorf("每条连线的两端均须填写有效的 agent_slug")
		}
		if err := validateCollabAgentSlugSyntax(a); err != nil {
			return err
		}
		if err := validateCollabAgentSlugSyntax(b); err != nil {
			return err
		}
		lo, hi := canonicalEdge(a, b)
		if lo == hi {
			return fmt.Errorf("不能将员工与自身连线：%q", lo)
		}
		if _, ok := slugSet[lo]; !ok {
			return fmt.Errorf("未知的 agent_slug %q（请先在员工列表中添加该 id）", lo)
		}
		if _, ok := slugSet[hi]; !ok {
			return fmt.Errorf("未知的 agent_slug %q（请先在员工列表中添加该 id）", hi)
		}
		key := lo + "\x00" + hi
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		uniq = append(uniq, [2]string{lo, hi})
	}

	if len(uniq) > MaxCollaborationEdgesPerInstance {
		return fmt.Errorf("连线数量过多（最多 %d 条）", MaxCollaborationEdgesPerInstance)
	}

	if _, err := tx.Exec(`DELETE FROM instance_topology_edges WHERE instance_id = ?`, instanceID); err != nil {
		return err
	}
	for _, e := range uniq {
		if _, err := tx.Exec(
			`INSERT INTO instance_topology_edges (instance_id, agent_slug_lo, agent_slug_hi) VALUES (?, ?, ?)`,
			instanceID, e[0], e[1],
		); err != nil {
			return err
		}
	}
	if err := d.bumpAgentTopologyVersion(tx, instanceID); err != nil {
		return err
	}
	return tx.Commit()
}

// ErrInternalMailListOffsetTooLarge 邮件列表 offset 超过 ListInternalMails 允许的上限
var ErrInternalMailListOffsetTooLarge = errors.New("邮件列表分页 offset 超过上限")

// ErrInternalMailInvalidThreadFilter 列表/统计的 thread_id 查询参数不合法（如超长）
var ErrInternalMailInvalidThreadFilter = errors.New("thread_id 筛选无效")

const (
	// MaxInternalMailListLimit ListInternalMails 单次请求最大条数
	MaxInternalMailListLimit = 500
	// MaxInternalMailListOffset ListInternalMails 允许的最大 offset
	MaxInternalMailListOffset = 500_000
)

// AreNeighbors 无向：检查是否存在边
func (d *DB) AreNeighbors(instanceID int64, a, b string) (bool, error) {
	if err := validateCollabAgentSlugSyntax(a); err != nil {
		return false, err
	}
	if err := validateCollabAgentSlugSyntax(b); err != nil {
		return false, err
	}
	lo, hi := canonicalEdge(a, b)
	var n int
	err := d.QueryRow(
		`SELECT COUNT(*) FROM instance_topology_edges WHERE instance_id = ? AND agent_slug_lo = ? AND agent_slug_hi = ?`,
		instanceID, lo, hi,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// validateInternalMailReplyAgainstParent 在已取到父邮件行后校验原路回复（供单测覆盖）。
func validateInternalMailReplyAgainstParent(parent *InternalMailRow, threadID, fromSlug, toSlug string) error {
	if parent == nil {
		return fmt.Errorf("in_reply_to：所引用的邮件不存在")
	}
	tid := strings.TrimSpace(threadID)
	if strings.TrimSpace(parent.ThreadID) != tid {
		return fmt.Errorf("in_reply_to：thread_id 与父邮件不一致")
	}
	fs := strings.TrimSpace(fromSlug)
	ts := strings.TrimSpace(toSlug)
	if strings.TrimSpace(parent.ToSlug) != fs {
		return fmt.Errorf("in_reply_to：只能由上一封邮件的收件人（%q）回复", parent.ToSlug)
	}
	if strings.TrimSpace(parent.FromSlug) != ts {
		return fmt.Errorf("in_reply_to：回复须发给原发件人（%q）", parent.FromSlug)
	}
	return nil
}

// ValidateInternalMailReply 当带 in_reply_to 时：父邮件须存在、同 thread，发件人须为父邮件收件人，收件人须为父邮件发件人（原路回复）。
func (d *DB) ValidateInternalMailReply(instanceID int64, threadID, fromSlug, toSlug string, inReplyTo *int64) error {
	if inReplyTo == nil || *inReplyTo < 1 {
		return nil
	}
	if _, err := d.ValidateInternalMailThreadForPost(threadID); err != nil {
		return err
	}
	parent, err := d.GetInternalMailByID(instanceID, *inReplyTo)
	if err != nil {
		return err
	}
	return validateInternalMailReplyAgainstParent(parent, threadID, fromSlug, toSlug)
}

const (
	// MaxInternalMailThreadRunes thread_id 最大 Unicode 字符数（与表 internal_mails.thread_id 一致）
	MaxInternalMailThreadRunes = 64
	// MaxInternalMailSubjectRunes 主题最大 Unicode 字符数（与表 subject 一致）
	MaxInternalMailSubjectRunes = 512
	// MaxInternalMailBodyBytes 正文最大字节数（UTF-8）
	MaxInternalMailBodyBytes = 256 * 1024
)

func validateInternalMailThreadID(threadID string) error {
	if utf8.RuneCountInString(threadID) > MaxInternalMailThreadRunes {
		return fmt.Errorf("thread_id 过长（最多 %d 字）", MaxInternalMailThreadRunes)
	}
	return nil
}

// ValidateInternalMailThreadForPost 发内部邮件前校验 thread_id：trim、非空、长度与表字段一致。
func (d *DB) ValidateInternalMailThreadForPost(raw string) (trimmed string, err error) {
	t := strings.TrimSpace(raw)
	if t == "" {
		return "", fmt.Errorf("thread_id 不能为空")
	}
	if err := validateInternalMailThreadID(t); err != nil {
		return "", err
	}
	return t, nil
}

func validateInternalMailContent(subject, body string) error {
	if utf8.RuneCountInString(subject) > MaxInternalMailSubjectRunes {
		return fmt.Errorf("主题过长（最多 %d 字）", MaxInternalMailSubjectRunes)
	}
	if len(body) > MaxInternalMailBodyBytes {
		return fmt.Errorf("正文过长（最多 %d KB）", MaxInternalMailBodyBytes/1024)
	}
	return nil
}

// InsertInternalMail 写入邮件；邻居关系由调用方校验
func (d *DB) InsertInternalMail(instanceID int64, threadID, fromSlug, toSlug, subject, body string, inReplyTo *int64) (int64, int64, error) {
	var err error
	threadID, err = d.ValidateInternalMailThreadForPost(threadID)
	if err != nil {
		return 0, 0, err
	}
	if err := validateInternalMailContent(subject, body); err != nil {
		return 0, 0, err
	}
	ver, err := d.GetAgentTopologyVersion(instanceID)
	if err != nil {
		return 0, 0, err
	}
	res, err := d.Exec(
		`INSERT INTO internal_mails (instance_id, thread_id, from_slug, to_slug, subject, body, in_reply_to, topology_version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		instanceID, threadID, strings.TrimSpace(fromSlug), strings.TrimSpace(toSlug), subject, body, nullableInt64(inReplyTo), ver,
	)
	if err != nil {
		return 0, 0, err
	}
	id, _ := res.LastInsertId()
	return id, ver, nil
}

func nullableInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// ListInternalMails 按实例列举，可选 thread_id；owner 校验在 handler
func (d *DB) ListInternalMails(instanceID int64, threadID string, limit, offset int) ([]InternalMailRow, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > MaxInternalMailListLimit {
		limit = MaxInternalMailListLimit
	}
	if offset < 0 {
		offset = 0
	}
	if offset > MaxInternalMailListOffset {
		return nil, fmt.Errorf("%w（最多 %d，建议使用 thread_id 筛选）", ErrInternalMailListOffsetTooLarge, MaxInternalMailListOffset)
	}
	tid := strings.TrimSpace(threadID)
	if tid != "" {
		if e := validateInternalMailThreadID(tid); e != nil {
			return nil, fmt.Errorf("%w: %v", ErrInternalMailInvalidThreadFilter, e)
		}
	}
	var rows *sql.Rows
	var err error
	if tid != "" {
		rows, err = d.Query(
			`SELECT id, instance_id, thread_id, from_slug, to_slug, subject, body, in_reply_to, topology_version,
				DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s') FROM internal_mails
			 WHERE instance_id = ? AND thread_id = ? ORDER BY id ASC LIMIT ? OFFSET ?`,
			instanceID, tid, limit, offset,
		)
	} else {
		rows, err = d.Query(
			`SELECT id, instance_id, thread_id, from_slug, to_slug, subject, body, in_reply_to, topology_version,
				DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s') FROM internal_mails
			 WHERE instance_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
			instanceID, limit, offset,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInternalMailRows(rows)
}

// CountInternalMails 符合条件的邮件总数（与 ListInternalMails 相同 instance / thread 条件）
func (d *DB) CountInternalMails(instanceID int64, threadID string) (int64, error) {
	tid := strings.TrimSpace(threadID)
	if tid != "" {
		if e := validateInternalMailThreadID(tid); e != nil {
			return 0, fmt.Errorf("%w: %v", ErrInternalMailInvalidThreadFilter, e)
		}
	}
	var n int64
	var err error
	if tid != "" {
		err = d.QueryRow(
			`SELECT COUNT(*) FROM internal_mails WHERE instance_id = ? AND thread_id = ?`,
			instanceID, tid,
		).Scan(&n)
	} else {
		err = d.QueryRow(`SELECT COUNT(*) FROM internal_mails WHERE instance_id = ?`, instanceID).Scan(&n)
	}
	return n, err
}

// GetInternalMailByID 单封邮件（容器拉正文）；必须属于该 instance
func (d *DB) GetInternalMailByID(instanceID, mailID int64) (*InternalMailRow, error) {
	var r InternalMailRow
	var inReply sql.NullInt64
	err := d.QueryRow(
		`SELECT id, instance_id, thread_id, from_slug, to_slug, subject, body, in_reply_to, topology_version,
			DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s') FROM internal_mails WHERE id = ? AND instance_id = ?`,
		mailID, instanceID,
	).Scan(&r.ID, &r.InstanceID, &r.ThreadID, &r.FromSlug, &r.ToSlug, &r.Subject, &r.Body, &inReply, &r.TopologyVersion, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if inReply.Valid {
		v := inReply.Int64
		r.InReplyTo = &v
	}
	return &r, nil
}

func scanInternalMailRows(rows *sql.Rows) ([]InternalMailRow, error) {
	var list []InternalMailRow
	for rows.Next() {
		var r InternalMailRow
		var inReply sql.NullInt64
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.ThreadID, &r.FromSlug, &r.ToSlug, &r.Subject, &r.Body, &inReply, &r.TopologyVersion, &r.CreatedAt); err != nil {
			return nil, err
		}
		if inReply.Valid {
			v := inReply.Int64
			r.InReplyTo = &v
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// collabResolveNameInput 指人解析查询串：trim、非空、长度与展示名字段一致
func collabResolveNameInput(raw string) (q string, err error) {
	q = strings.TrimSpace(raw)
	if q == "" {
		return "", fmt.Errorf("名称不能为空")
	}
	if utf8.RuneCountInString(q) > MaxInstanceAgentDisplayRunes {
		return "", fmt.Errorf("名称过长（最多 %d 字）", MaxInstanceAgentDisplayRunes)
	}
	return q, nil
}

// ResolveDisplayNameForInstance 自然语言指人：先精确再前缀；仅本实例 roster
func (d *DB) ResolveDisplayNameForInstance(instanceID, userID int64, raw string) (slug string, ambiguous []string, err error) {
	q, err := collabResolveNameInput(raw)
	if err != nil {
		return "", nil, err
	}
	// 精确（依赖 utf8mb4_unicode_ci 时大小写不敏感）
	var exact string
	err = d.QueryRow(
		`SELECT agent_slug FROM instance_agents WHERE instance_id = ? AND user_id = ? AND display_name = ? LIMIT 2`,
		instanceID, userID, q,
	).Scan(&exact)
	if err == nil {
		return exact, nil, nil
	}
	if err != sql.ErrNoRows {
		return "", nil, err
	}
	// 前缀匹配（可能多条）
	rows, err := d.Query(
		`SELECT agent_slug, display_name FROM instance_agents WHERE instance_id = ? AND user_id = ? AND display_name LIKE ?`,
		instanceID, userID, q+"%",
	)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var s, dn string
		if err := rows.Scan(&s, &dn); err != nil {
			return "", nil, err
		}
		slugs = append(slugs, s)
		ambiguous = append(ambiguous, dn)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	if len(slugs) == 1 {
		return slugs[0], nil, nil
	}
	if len(slugs) == 0 {
		return "", nil, nil
	}
	sort.Strings(ambiguous)
	return "", ambiguous, nil // 多条前缀命中，需消歧
}

// validateCollabAgentSlugSyntax 与 instance_agents.agent_slug 字段规则一致（容器发内部邮件等）
func validateCollabAgentSlugSyntax(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		return fmt.Errorf("agent_slug 不能为空")
	}
	if utf8.RuneCountInString(s) > MaxInstanceAgentSlugRunes {
		return fmt.Errorf("agent_slug 过长（最多 %d 字）", MaxInstanceAgentSlugRunes)
	}
	return nil
}

// VerifySlugsBelongToInstance 发信前校验双方 slug 属于该实例
func (d *DB) VerifySlugsBelongToInstance(instanceID int64, slugs ...string) error {
	for _, raw := range slugs {
		if err := validateCollabAgentSlugSyntax(raw); err != nil {
			return err
		}
		s := strings.TrimSpace(raw)
		var n int
		err := d.QueryRow(`SELECT COUNT(*) FROM instance_agents WHERE instance_id = ? AND agent_slug = ?`, instanceID, s).Scan(&n)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("本实例不存在 agent_slug %q（请与协作员工列表对齐）", s)
		}
	}
	return nil
}
