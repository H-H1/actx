package p2c

import (
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Node 表示一个服务节点
type Node struct {
	// 节点标识
	ID string
	// 当前负载（正在处理的请求数）
	Load int64
	// 节点权重（静态权重）
	Weight int
	// 上次使用时间（用于冷却机制）
	LastUsed time.Time
	// 失败计数
	FailCount int64
	// 是否健康
	Healthy bool
	mu      sync.RWMutex
}

// NewNode 创建一个新的节点
func NewNode(id string, weight int) *Node {
	return &Node{
		ID:        id,
		Weight:    weight,
		Load:      0,
		Healthy:   true,
		LastUsed:  time.Now(),
		FailCount: 0,
	}
}

// IncrementLoad 增加负载
func (n *Node) IncrementLoad() {
	atomic.AddInt64(&n.Load, 1)
}

// DecrementLoad 减少负载
func (n *Node) DecrementLoad() {
	atomic.AddInt64(&n.Load, -1)
}

// GetLoad 获取当前负载
func (n *Node) GetLoad() int64 {
	return atomic.LoadInt64(&n.Load)
}

// MarkSuccess 标记成功
func (n *Node) MarkSuccess() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.FailCount = 0
	n.Healthy = true
	n.LastUsed = time.Now()
}

// MarkFailure 标记失败
func (n *Node) MarkFailure() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.FailCount++
	// 连续失败超过5次，标记为不健康
	if n.FailCount >= 5 {
		n.Healthy = false
	}
	n.LastUsed = time.Now()
}

// IsHealthy 检查节点是否健康
func (n *Node) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Healthy
}

// GetLoadScore 计算负载评分（综合考虑负载和权重）
func (n *Node) GetLoadScore() float64 {
	load := float64(atomic.LoadInt64(&n.Load))
	if n.Weight > 0 {
		return load / float64(n.Weight)
	}
	return load
}

// P2CSelector P2C选择器
type P2CSelector struct {
	nodes []*Node
	mu    sync.RWMutex
	rng   *rand.Rand
	// 冷却时间：避免频繁选择同一个节点
	CoolDownTime time.Duration
	// 探活间隔
	HealthCheckInterval time.Duration
	// 是否启用指数加权移动平均（EWMA）
	EnableEWMA bool
	// EWMA权重因子（0-1之间，默认0.5）
	EWMAFactor float64
	// 节点负载的EWMA值
	ewmaValues map[string]*ewmaValue
}

// ewmaValue 指数加权移动平均值
type ewmaValue struct {
	value float64
	mu    sync.RWMutex
}

func newEWMAValue(initial float64) *ewmaValue {
	return &ewmaValue{
		value: initial,
	}
}

func (e *ewmaValue) Update(newValue float64, factor float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.value = e.value*(1-factor) + newValue*factor
}

func (e *ewmaValue) Get() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.value
}

// NewP2CSelector 创建P2C选择器
func NewP2CSelector() *P2CSelector {
	return &P2CSelector{
		nodes:               make([]*Node, 0),
		rng:                 rand.New(rand.NewSource(time.Now().UnixNano())),
		CoolDownTime:        5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		EnableEWMA:          false,
		EWMAFactor:          0.5,
		ewmaValues:          make(map[string]*ewmaValue),
	}
}

// AddNode 添加节点
func (s *P2CSelector) AddNode(node *Node) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已存在
	for _, n := range s.nodes {
		if n.ID == node.ID {
			return
		}
	}
	s.nodes = append(s.nodes, node)

	// 初始化EWMA
	if s.EnableEWMA {
		s.ewmaValues[node.ID] = newEWMAValue(0)
	}
}

// RemoveNode 移除节点
func (s *P2CSelector) RemoveNode(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, n := range s.nodes {
		if n.ID == nodeID {
			s.nodes = append(s.nodes[:i], s.nodes[i+1:]...)
			delete(s.ewmaValues, nodeID)
			return
		}
	}
}

// GetHealthyNodes 获取所有健康节点
func (s *P2CSelector) GetHealthyNodes() []*Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	healthy := make([]*Node, 0)
	for _, n := range s.nodes {
		if n.IsHealthy() {
			healthy = append(healthy, n)
		}
	}
	return healthy
}

// Select 选择一个节点
func (s *P2CSelector) Select() (*Node, error) {
	s.mu.RLock()
	healthyNodes := make([]*Node, 0)
	for _, n := range s.nodes {
		if n.IsHealthy() {
			healthyNodes = append(healthyNodes, n)
		}
	}
	s.mu.RUnlock()

	if len(healthyNodes) == 0 {
		return nil, errors.New("no healthy nodes available")
	}

	if len(healthyNodes) == 1 {
		return healthyNodes[0], nil
	}

	// 随机选择两个不同的节点
	idx1 := s.rng.Intn(len(healthyNodes))
	idx2 := s.rng.Intn(len(healthyNodes))

	// 确保选择两个不同的节点
	for idx1 == idx2 && len(healthyNodes) > 1 {
		idx2 = s.rng.Intn(len(healthyNodes))
	}

	node1 := healthyNodes[idx1]
	node2 := healthyNodes[idx2]

	// 检查冷却时间
	now := time.Now()
	if now.Sub(node1.LastUsed) < s.CoolDownTime && now.Sub(node2.LastUsed) < s.CoolDownTime {
		// 如果两个都在冷却中，随机选择一个
		if s.rng.Intn(2) == 0 {
			return node1, nil
		}
		return node2, nil
	}

	// 计算负载分数
	score1 := s.calculateScore(node1)
	score2 := s.calculateScore(node2)

	// 选择负载较低的节点
	if score1 <= score2 {
		if now.Sub(node1.LastUsed) >= s.CoolDownTime {
			return node1, nil
		}
		return node2, nil
	}

	if now.Sub(node2.LastUsed) >= s.CoolDownTime {
		return node2, nil
	}
	return node1, nil
}

// calculateScore 计算节点评分
func (s *P2CSelector) calculateScore(node *Node) float64 {
	var score float64

	if s.EnableEWMA {
		// 使用EWMA计算负载
		ewma := s.ewmaValues[node.ID]
		if ewma != nil {
			score = ewma.Get()
			// 更新EWMA
			currentLoad := float64(node.GetLoad())
			ewma.Update(currentLoad, s.EWMAFactor)
		} else {
			score = float64(node.GetLoad())
		}
	} else {
		score = node.GetLoadScore()
	}

	return score
}

// UpdateNodeLoad 更新节点负载（请求开始时调用）
func (s *P2CSelector) UpdateNodeLoad(node *Node, increment bool) {
	if increment {
		node.IncrementLoad()
	} else {
		node.DecrementLoad()
	}

	// 更新EWMA
	if s.EnableEWMA {
		ewma := s.ewmaValues[node.ID]
		if ewma != nil {
			currentLoad := float64(node.GetLoad())
			ewma.Update(currentLoad, s.EWMAFactor)
		}
	}
}

// MarkNodeSuccess 标记节点成功
func (s *P2CSelector) MarkNodeSuccess(node *Node) {
	node.MarkSuccess()
}

// MarkNodeFailure 标记节点失败
func (s *P2CSelector) MarkNodeFailure(node *Node) {
	node.MarkFailure()
}

// GetStats 获取统计信息
func (s *P2CSelector) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})
	nodeStats := make([]map[string]interface{}, 0)

	for _, n := range s.nodes {
		nodeStat := map[string]interface{}{
			"id":        n.ID,
			"load":      n.GetLoad(),
			"weight":    n.Weight,
			"healthy":   n.IsHealthy(),
			"failCount": n.FailCount,
			"lastUsed":  n.LastUsed,
		}

		if s.EnableEWMA {
			if ewma, ok := s.ewmaValues[n.ID]; ok {
				nodeStat["ewma"] = ewma.Get()
			}
		}

		nodeStats = append(nodeStats, nodeStat)
	}

	stats["nodes"] = nodeStats
	stats["totalNodes"] = len(s.nodes)
	stats["healthyNodes"] = len(s.GetHealthyNodes())
	stats["enableEWMA"] = s.EnableEWMA

	return stats
}
