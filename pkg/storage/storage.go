// Package storage 统一文件存储抽象：本地磁盘 / 阿里云 OSS。
//
// 脚手架原始版本缺少该包（.gitignore 未忽略但未入库），这里按 service 层实际调用补齐：
// Put / Delete / URL / Type / Exists / PresignPut。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"web-basic/internal/config"
)

// ErrDirectUploadUnsupported 当前存储不支持前端直传（本地磁盘）。
var ErrDirectUploadUnsupported = errors.New("当前存储不支持前端直传")

// Storage 文件存储接口。
type Storage interface {
	// Put 写入对象并返回可访问地址。
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error)
	// Delete 删除对象（不存在时不应报错）。
	Delete(ctx context.Context, key string) error
	// URL 返回访问地址；ttl>0 时尽量返回临时签名地址。
	URL(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Type 返回 local / oss。
	Type() string
	// Exists 判断对象是否存在。
	Exists(ctx context.Context, key string) (bool, error)
	// PresignPut 生成前端直传签名（仅 OSS）。
	PresignPut(ctx context.Context, key string, expire time.Duration, contentType string) (string, error)
}

// New 按配置创建存储实现。
func New(cfg config.Storage) (Storage, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "", "local":
		return newLocal(cfg.Local)
	case "oss":
		return newOSS(cfg.OSS)
	default:
		return nil, fmt.Errorf("不支持的存储类型：%s（可选 local / oss）", cfg.Type)
	}
}

// safePath 防目录穿越：对象键只允许相对路径。
func safePath(base, key string) (string, error) {
	key = strings.TrimPrefix(strings.ReplaceAll(key, "\\", "/"), "/")
	if key == "" || strings.Contains(key, "..") {
		return "", errors.New("非法对象键")
	}
	p := filepath.Join(base, filepath.FromSlash(key))
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if absP != absBase && !strings.HasPrefix(absP, absBase+string(os.PathSeparator)) {
		return "", errors.New("对象键越界")
	}
	return absP, nil
}

// localStore 本地磁盘存储。
type localStore struct {
	baseDir   string
	urlPrefix string
}

func newLocal(cfg config.LocalStore) (Storage, error) {
	base := cfg.BaseDir
	if base == "" {
		base = "./storage"
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("创建本地存储目录失败: %w", err)
	}
	prefix := strings.TrimSuffix(cfg.URLPrefix, "/")
	if prefix == "" {
		prefix = "/static"
	}
	return &localStore{baseDir: base, urlPrefix: prefix}, nil
}

func (s *localStore) Type() string { return "local" }

func (s *localStore) Put(_ context.Context, key string, r io.Reader, size int64, _ string) (string, error) {
	p, err := safePath(s.baseDir, key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if size > 0 {
		if _, err := io.CopyN(f, r, size); err != nil {
			_ = os.Remove(p)
			return "", err
		}
	} else if _, err := io.Copy(f, r); err != nil {
		_ = os.Remove(p)
		return "", err
	}
	return s.URL(context.Background(), key, 0)
}

func (s *localStore) Delete(_ context.Context, key string) error {
	p, err := safePath(s.baseDir, key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *localStore) URL(_ context.Context, key string, _ time.Duration) (string, error) {
	key = strings.TrimPrefix(strings.ReplaceAll(key, "\\", "/"), "/")
	return s.urlPrefix + "/" + key, nil
}

func (s *localStore) Exists(_ context.Context, key string) (bool, error) {
	p, err := safePath(s.baseDir, key)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *localStore) PresignPut(context.Context, string, time.Duration, string) (string, error) {
	return "", ErrDirectUploadUnsupported
}

// ossStore 阿里云 OSS 存储。
type ossStore struct {
	bucket *oss.Bucket
	cfg    config.OSSStore
}

func newOSS(cfg config.OSSStore) (Storage, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("OSS 存储需要配置 endpoint 与 bucket")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("OSS 存储需要配置 access_key / secret_key")
	}
	client, err := oss.New(cfg.Endpoint, cfg.AccessKey, cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("初始化 OSS 客户端失败: %w", err)
	}
	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("获取 OSS Bucket 失败: %w", err)
	}
	return &ossStore{bucket: bucket, cfg: cfg}, nil
}

func (s *ossStore) Type() string { return "oss" }

func (s *ossStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	opts := []oss.Option{}
	if contentType != "" {
		opts = append(opts, oss.ContentType(contentType))
	}
	if err := s.bucket.PutObject(key, r, opts...); err != nil {
		return "", err
	}
	return s.URL(ctx, key, 0)
}

func (s *ossStore) Delete(_ context.Context, key string) error {
	err := s.bucket.DeleteObject(key)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "nosuchkey") {
		return nil
	}
	return err
}

func (s *ossStore) URL(_ context.Context, key string, ttl time.Duration) (string, error) {
	key = strings.TrimPrefix(key, "/")
	if s.cfg.Private && ttl > 0 {
		return s.bucket.SignURL(key, oss.HTTPGet, int64(ttl.Seconds()))
	}
	if s.cfg.Domain != "" {
		return strings.TrimSuffix(s.cfg.Domain, "/") + "/" + key, nil
	}
	host := strings.TrimPrefix(strings.TrimPrefix(s.cfg.Endpoint, "https://"), "http://")
	scheme := "https://"
	if strings.HasPrefix(s.cfg.Endpoint, "http://") {
		scheme = "http://"
	}
	return scheme + s.cfg.Bucket + "." + host + "/" + key, nil
}

func (s *ossStore) Exists(_ context.Context, key string) (bool, error) {
	return s.bucket.IsObjectExist(key)
}

func (s *ossStore) PresignPut(_ context.Context, key string, expire time.Duration, _ string) (string, error) {
	if expire <= 0 {
		expire = 15 * time.Minute
	}
	return s.bucket.SignURL(key, oss.HTTPPut, int64(expire.Seconds()))
}
