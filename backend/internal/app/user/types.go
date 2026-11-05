package user

// ===== 演示接口 GET /api/from/:name（验证前后端连通）=====

type Request struct {
	Name string `path:"name,options=you|me"`
}

type Response struct {
	Message string `json:"message"`
}

// ===== 登录接口 POST /api/user/login =====

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Role     string `json:"role"` // admin/teacher/student，前端据此控制创建班级等入口
	// MustChangePassword 初始账号 / 管理员重置密码后为 true，前端应立刻引导改密。
	MustChangePassword bool `json:"mustChangePassword"`
}

// ===== 注册接口 POST /api/user/register =====

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterResponse struct {
	Username string `json:"username"`
}

// ===== 登出接口 POST /api/user/logout（需登录，token 放在 Authorization 头） =====

type LogoutResponse struct {
	Username string `json:"username"`
}

// ===== 当前用户信息接口 GET /api/user/info（需登录） =====

type InfoResponse struct {
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"` // admin/teacher/student
	// MustChangePassword 是否需要先改密（前端顶栏常驻提示）。
	MustChangePassword bool `json:"mustChangePassword"`
}

// ===== 修改密码接口 POST /api/user/password（需登录） =====

// ChangePasswordRequest 用户自己改密：必须提供旧密码，防止 token 被盗后直接改密。
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// ChangePasswordResponse 改密结果。
type ChangePasswordResponse struct {
	Username string `json:"username"`
	// MustChangePassword 改密成功后恒为 false，前端据此收起引导。
	MustChangePassword bool `json:"mustChangePassword"`
}
