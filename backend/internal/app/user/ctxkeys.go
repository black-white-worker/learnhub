package user

// ctxKey 自定义上下文键类型，避免与其他包使用的键冲突。
type ctxKey int

const (
	// authTokenKey 鉴权中间件放入 context 的登录 token
	authTokenKey ctxKey = iota
	// authSessionKey 鉴权中间件放入 context 的会话信息
	authSessionKey
	// clientIPKey 注册/登录等场景放入 context 的客户端 IP（用于限流）
	clientIPKey
)
