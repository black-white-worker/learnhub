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
}
