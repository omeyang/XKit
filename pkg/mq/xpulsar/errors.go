package xpulsar

import (
	"errors"

	"github.com/omeyang/xkit/internal/mqcore"
)

// 共享错误（从 mqcore 重导出）
var (
	// ErrNilClient 客户端为 nil 错误
	ErrNilClient = mqcore.ErrNilClient

	// ErrNilMessage 消息为 nil 错误
	ErrNilMessage = mqcore.ErrNilMessage

	// ErrNilHandler 处理函数为 nil 错误
	ErrNilHandler = mqcore.ErrNilHandler
)

// Pulsar 特定错误
var (
	// ErrEmptyURL URL 为空错误
	ErrEmptyURL = errors.New("xpulsar: empty URL")

	// ErrNilOption 选项函数为 nil 错误
	ErrNilOption = errors.New("xpulsar: nil option")

	// ErrNilProducer 生产者为 nil 错误
	ErrNilProducer = errors.New("xpulsar: nil producer")

	// ErrNilConsumer 消费者为 nil 错误
	ErrNilConsumer = errors.New("xpulsar: nil consumer")

	// ErrClosed 客户端已关闭错误（复用 mqcore.ErrClosed，与 xkafka 对齐）
	ErrClosed = mqcore.ErrClosed
)

// Topic 解析错误
var (
	// ErrEmptyTopicURI Topic URI 为空
	ErrEmptyTopicURI = errors.New("xpulsar: empty topic URI")

	// ErrInvalidTopicScheme Topic URI scheme 无效（必须为 persistent:// 或 non-persistent://）
	ErrInvalidTopicScheme = errors.New("xpulsar: invalid topic scheme")

	// ErrInvalidTopicFormat Topic URI 格式不正确（必须包含 tenant/namespace/topic 三段）
	ErrInvalidTopicFormat = errors.New("xpulsar: invalid topic format")

	// ErrInvalidTenant tenant 名称无效
	ErrInvalidTenant = errors.New("xpulsar: invalid tenant")

	// ErrInvalidNamespace namespace 名称无效
	ErrInvalidNamespace = errors.New("xpulsar: invalid namespace")

	// ErrInvalidTopicName topic 名称无效
	ErrInvalidTopicName = errors.New("xpulsar: invalid topic name")
)

// Auth 错误
var (
	// ErrEmptyToken token 为空
	ErrEmptyToken = errors.New("xpulsar: empty token")

	// ErrEmptyTokenFilePath token 文件路径为空
	ErrEmptyTokenFilePath = errors.New("xpulsar: empty token file path")

	// ErrEmptyCertPath 证书路径为空
	ErrEmptyCertPath = errors.New("xpulsar: empty certificate path")

	// ErrEmptyKeyPath 私钥路径为空
	ErrEmptyKeyPath = errors.New("xpulsar: empty key path")

	// ErrNilSupplier supplier 函数为 nil
	ErrNilSupplier = errors.New("xpulsar: nil supplier function")

	// ErrEmptyIssuerURL OAuth2 issuer URL 为空
	ErrEmptyIssuerURL = errors.New("xpulsar: empty issuer URL")

	// ErrEmptyAudience OAuth2 audience 为空
	ErrEmptyAudience = errors.New("xpulsar: empty audience")

	// ErrEmptyCredentialsPath OAuth2 credentials 文件路径为空
	ErrEmptyCredentialsPath = errors.New("xpulsar: empty credentials path")

	// ErrEmptyAuthParams 认证参数为 nil 或空 map
	ErrEmptyAuthParams = errors.New("xpulsar: empty auth params")

	// ErrEmptyEnvKey 环境变量名为空
	ErrEmptyEnvKey = errors.New("xpulsar: empty env key")
)
