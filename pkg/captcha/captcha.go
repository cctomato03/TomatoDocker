// Package captcha 图形验证码：base64 输出 + 进程内存储（不再依赖 Redis）。
package captcha

import (
	"sync"
	"time"

	"github.com/mojocn/base64Captcha"
)

// MemoryStore 验证码存储实现（base64Captcha.Store）。
//
// 验证码是短生命周期的一次性数据，单容器部署下放内存即可；
// 进程重启后旧验证码失效是预期行为。
type MemoryStore struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]memoryItem
}

type memoryItem struct {
	value   string
	expires time.Time
}

// NewMemoryStore 创建内存验证码存储。
func NewMemoryStore(ttl time.Duration) *MemoryStore {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &MemoryStore{ttl: ttl, items: make(map[string]memoryItem)}
}

func (s *MemoryStore) Set(id, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, it := range s.items {
		if now.After(it.expires) {
			delete(s.items, k)
		}
	}
	s.items[id] = memoryItem{value: value, expires: now.Add(s.ttl)}
	return nil
}

func (s *MemoryStore) Get(id string, clear bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok || time.Now().After(it.expires) {
		delete(s.items, id)
		return ""
	}
	if clear {
		delete(s.items, id)
	}
	return it.value
}

func (s *MemoryStore) Verify(id, answer string, clear bool) bool {
	want := s.Get(id, clear)
	return want != "" && want == answer
}

// Service 验证码服务。
type Service struct {
	captcha *base64Captcha.Captcha
	enabled bool
}

// NewService 创建验证码服务。
func NewService(store base64Captcha.Store, enabled bool) *Service {
	// 数字+字母混合，宽 240 高 80，长度 5，干扰强度 0.6
	driver := base64Captcha.NewDriverString(80, 240, 0, base64Captcha.OptionShowSlimeLine, 5,
		"23456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ", nil, nil, nil)
	return &Service{captcha: base64Captcha.NewCaptcha(driver, store), enabled: enabled}
}

// Enabled 是否开启验证码。
func (s *Service) Enabled() bool { return s.enabled }

// Generate 生成验证码，返回 id 与 base64 图片（data:image/png;base64,...）。
func (s *Service) Generate() (id, image string, err error) {
	id, image, _, err = s.captcha.Generate()
	return
}

// Verify 校验验证码（一次性）。
func (s *Service) Verify(id, answer string) bool {
	if !s.enabled {
		return true
	}
	if id == "" || answer == "" {
		return false
	}
	return s.captcha.Verify(id, answer, true)
}
