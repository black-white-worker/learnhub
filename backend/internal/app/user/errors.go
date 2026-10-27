package user

// 认证相关的业务错误：带 HTTP 状态码，交给 response.WriteError 输出。
//
// 为什么要专门定义：登录失败原来用 errors.New 一路抛到 httpx.ErrorCtx，
// 一律变成 500「服务器错误」，前端没法区分"密码错了"和"被限流了"。
// 这里定义成 401 / 429，语义就和真实原因对齐了。

// AuthError 认证错误。
type AuthError struct {
	Code int
	Msg  string
}

func (e *AuthError) Error() string   { return e.Msg }
func (e *AuthError) StatusCode() int { return e.Code }

// ErrBadCredentials 用户名或密码错误（401）。
var ErrBadCredentials = &AuthError{Code: 401, Msg: "用户名或密码错误"}

// ErrTooManyAttempts 登录失败次数过多（429），锁定 10 分钟。
var ErrTooManyAttempts = &AuthError{Code: 429, Msg: "登录失败次数过多，请 10 分钟后再试"}

// NewAuthError 构造一个带状态码的认证/账号错误（改密、重置密码等场景用）。
func NewAuthError(code int, msg string) *AuthError {
	return &AuthError{Code: code, Msg: msg}
}
