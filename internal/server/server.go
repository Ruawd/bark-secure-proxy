package server

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bark-labs/bark-secure-proxy/internal/barkclient"
	"github.com/bark-labs/bark-secure-proxy/internal/config"
	"github.com/bark-labs/bark-secure-proxy/internal/model"
	"github.com/bark-labs/bark-secure-proxy/internal/service"
	"github.com/bark-labs/bark-secure-proxy/internal/storage"
	"github.com/gofiber/fiber/v2"
)

// Server wires HTTP handlers.
type Server struct {
	app        *fiber.App
	deviceSvc  *service.DeviceService
	noticeSvc  *service.NoticeService
	logSvc     *service.NoticeLogService
	barkClient *barkclient.Client
	authSvc    *service.AuthService
	store      storage.Store
	cfg        *config.Config
}

// New builds a server instance.
func New(cfg *config.Config, store storage.Store, deviceSvc *service.DeviceService, noticeSvc *service.NoticeService, logSvc *service.NoticeLogService, authSvc *service.AuthService, barkClient *barkclient.Client) *Server {
	app := fiber.New(fiber.Config{
		IdleTimeout:  cfg.HTTP.ReadTimeout,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		AppName:      "bark-secure-proxy",
	})
	s := &Server{
		app:        app,
		deviceSvc:  deviceSvc,
		noticeSvc:  noticeSvc,
		logSvc:     logSvc,
		barkClient: barkClient,
		authSvc:    authSvc,
		store:      store,
		cfg:        cfg,
	}
	s.registerRoutes()
	return s
}

// Start listens and serves HTTP traffic.
func (s *Server) Start() error {
	return s.app.Listen(s.cfg.HTTP.Addr)
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.app.ShutdownWithContext(ctx)
}

func (s *Server) registerRoutes() {
	s.app.Get("/healthz", s.handleHealth)
	s.app.Get("/ping", s.handlePingProxy)

	s.app.Post("/auth/login", s.handleLogin)
	s.app.Get("/auth/profile", s.handleProfile)

	// Bark-App compatible endpoints
	s.app.Get("/register", s.handleRegister)
	s.app.Post("/device/gen", s.handleDeviceGen)
	s.app.Get("/device/query", s.handleDeviceQuery)
	s.app.Get("/device/queryAll", s.handleDeviceQueryAll)
	s.app.Get("/device/active", s.handleDeviceActivate)
	s.app.Get("/device/stop", s.handleDeviceStop)

	s.app.Get("/notice", s.handleNoticeQuery)
	s.app.Get("/notice/:title/:body", s.handleNoticePath)
	s.app.Get("/notice/:title/:subtitle/:body", s.handleNoticePath)
	s.app.Post("/notice", s.handleNoticePost)

	s.app.Get("/status/endpoint", s.handleStatusEndpoint)

	// Notice log APIs
	logGroup := s.app.Group("/api/notice/log", s.requireAuth)
	logGroup.Get("/list", s.handleLogList)
	logGroup.Get("/count/date", s.handleLogCountDate)
	logGroup.Get("/count/status", s.handleLogCountStatus)
	logGroup.Get("/count/group", s.handleLogCountGroup)
	logGroup.Get("/count/device", s.handleLogCountDevice)

	// Internal admin helpers for the lightweight frontend
	admin := s.app.Group("/admin", s.requireAuth)
	admin.Get("/summary", s.handleAdminSummary)
	admin.Get("/devices", s.handleAdminListDevices)
	admin.Get("/devices/:token", s.handleAdminGetDevice)
	admin.Post("/devices", s.handleAdminUpsertDevice)

	s.serveFrontend()
}

func (s *Server) handleHealth(c *fiber.Ctx) error {
	resp := fiber.Map{"status": "ok"}
	if s.barkClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := s.barkClient.Ping(ctx); err != nil {
			resp["bark"] = fiber.Map{"status": "degraded", "error": err.Error()}
		} else {
			resp["bark"] = fiber.Map{"status": "up"}
		}
	}
	return c.Status(http.StatusOK).JSON(resp)
}

func (s *Server) handlePingProxy(c *fiber.Ctx) error {
	if s.barkClient == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(model.Error("bark client not configured"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := s.barkClient.Ping(ctx)
	if err != nil {
		return c.Status(http.StatusBadGateway).JSON(model.Error(err.Error()))
	}
	return c.JSON(resp)
}

func (s *Server) handleLogin(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.JSON(model.Error("参数格式错误"))
	}
	if s.authSvc == nil || !s.authSvc.Enabled() {
		return c.JSON(model.Success("无需登录", fiber.Map{
			"token":    "",
			"enabled":  false,
			"username": "guest",
		}))
	}
	token, err := s.authSvc.Authenticate(req.Username, req.Password)
	if err != nil {
		return c.Status(http.StatusUnauthorized).JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("登录成功", fiber.Map{
		"token":    token,
		"enabled":  true,
		"username": s.authSvcUsername(),
	}))
}

func (s *Server) handleProfile(c *fiber.Ctx) error {
	if s.authSvc == nil || !s.authSvc.Enabled() {
		return c.JSON(model.Success("ok", fiber.Map{
			"enabled":  false,
			"username": "guest",
		}))
	}
	token := extractBearerToken(c.Get("Authorization"))
	if token == "" {
		return c.Status(http.StatusUnauthorized).JSON(model.Error("未登录"))
	}
	claims, err := s.authSvc.Validate(token)
	if err != nil {
		return c.Status(http.StatusUnauthorized).JSON(model.Error("登录已失效"))
	}
	return c.JSON(model.Success("ok", fiber.Map{
		"enabled":  true,
		"username": claims.Username,
	}))
}

func (s *Server) handleRegister(c *fiber.Ctx) error {
	if s.barkClient == nil {
		return c.JSON(model.Error("bark client not configured"))
	}
	deviceToken := c.Query("devicetoken")
	key := c.Query("key")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := s.deviceSvc.RegisterDevice(ctx, deviceToken, key)
	if err != nil {
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(resp)
}

func (s *Server) handleDeviceGen(c *fiber.Ctx) error {
	var req service.DeviceRequest
	if err := c.BodyParser(&req); err != nil {
		return c.JSON(model.Error("参数格式错误"))
	}
	device, err := s.deviceSvc.GenerateConfig(context.Background(), req)
	if err != nil {
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("添加成功", device))
}

func (s *Server) handleDeviceQuery(c *fiber.Ctx) error {
	token := c.Query("deviceToken")
	if token == "" {
		return c.JSON(model.Error("deviceToken不能为空"))
	}
	device, err := s.deviceSvc.Get(context.Background(), token)
	if err != nil {
		if err == storage.ErrNotFound {
			return c.JSON(model.Error("设备不存在"))
		}
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("查询成功", device))
}

func (s *Server) handleDeviceQueryAll(c *fiber.Ctx) error {
	views, err := s.deviceSvc.ListViews(context.Background())
	if err != nil {
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("查询成功", views))
}

func (s *Server) handleDeviceActivate(c *fiber.Ctx) error {
	return s.handleDeviceStatusChange(c, model.DeviceStatusActive, "激活成功")
}

func (s *Server) handleDeviceStop(c *fiber.Ctx) error {
	return s.handleDeviceStatusChange(c, model.DeviceStatusStop, "禁止成功")
}

func (s *Server) handleDeviceStatusChange(c *fiber.Ctx, status, msg string) error {
	token := c.Query("deviceToken")
	if token == "" {
		return c.JSON(model.Error("deviceToken不能为空"))
	}
	_, err := s.deviceSvc.UpdateStatus(context.Background(), token, status)
	if err != nil {
		if err == storage.ErrNotFound {
			return c.JSON(model.Error("设备不存在"))
		}
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success(msg, nil))
}

func (s *Server) handleNoticeQuery(c *fiber.Ctx) error {
	req := model.NoticeRequest{
		Title:    c.Query("title"),
		Subtitle: c.Query("subtitle"),
		Body:     c.Query("body"),
		Group:    c.Query("group"),
		Url:      c.Query("url"),
	}
	if strings.TrimSpace(req.Body) == "" {
		return c.JSON(model.Error("body不能为空"))
	}
	return s.dispatchNotice(c, req)
}

func (s *Server) handleNoticePath(c *fiber.Ctx) error {
	req := model.NoticeRequest{
		Title:    decodePathSegment(c.Params("title")),
		Subtitle: decodePathSegment(c.Params("subtitle")),
		Body:     decodePathSegment(c.Params("body")),
		Group:    c.Query("group"),
		Url:      c.Query("url"),
	}
	if strings.TrimSpace(req.Body) == "" {
		return c.JSON(model.Error("body不能为空"))
	}
	return s.dispatchNotice(c, req)
}

func (s *Server) handleNoticePost(c *fiber.Ctx) error {
	var req model.NoticeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.JSON(model.Error("请求格式错误"))
	}
	return s.dispatchNotice(c, req)
}

func (s *Server) dispatchNotice(c *fiber.Ctx, req model.NoticeRequest) error {
	summary, _, err := s.noticeSvc.Broadcast(context.Background(), req)
	if err != nil {
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("发送成功", summary))
}

func (s *Server) handleStatusEndpoint(c *fiber.Ctx) error {
	token := c.Get("API-TOKEN")
	expected := strings.TrimSpace(s.cfg.Bark.Token)
	if expected == "" || token != expected {
		return c.JSON(model.StatusRes{
			Status:          "未授权",
			ActiveDeviceNum: 0,
			AllDeviceNum:    0,
		})
	}
	devices, err := s.deviceSvc.List(context.Background())
	if err != nil {
		return c.JSON(model.StatusRes{
			Status:          "异常",
			ActiveDeviceNum: 0,
			AllDeviceNum:    0,
		})
	}
	active := 0
	for _, d := range devices {
		if strings.EqualFold(d.Status, model.DeviceStatusActive) || d.Status == "" {
			active++
		}
	}
	status := "离线"
	if s.pingBark() {
		status = "在线"
	}
	return c.JSON(model.StatusRes{
		Status:          status,
		ActiveDeviceNum: active,
		AllDeviceNum:    len(devices),
	})
}

func (s *Server) pingBark() bool {
	if s.barkClient == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := s.barkClient.Ping(ctx)
	return err == nil && resp != nil && resp.Code == http.StatusOK
}

func (s *Server) handleLogList(c *fiber.Ctx) error {
	filter := parseLogFilter(c)
	page, err := s.logSvc.Query(context.Background(), filter)
	if err != nil {
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("获取日志列表成功", page))
}

func (s *Server) handleLogCountDate(c *fiber.Ctx) error {
	begin, end := parseTimeRange(c)
	dateType := c.Query("dateType", "day")
	data, err := s.logSvc.CountByDate(context.Background(), dateType, begin, end)
	if err != nil {
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("按日期统计成功", data))
}

func (s *Server) handleLogCountStatus(c *fiber.Ctx) error {
	begin, end := parseTimeRange(c)
	data, err := s.logSvc.CountByStatus(context.Background(), begin, end)
	if err != nil {
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("按状态统计成功", data))
}

func (s *Server) handleLogCountGroup(c *fiber.Ctx) error {
	begin, end := parseTimeRange(c)
	data, err := s.logSvc.CountByGroup(context.Background(), begin, end)
	if err != nil {
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("按分组统计成功", data))
}

func (s *Server) handleLogCountDevice(c *fiber.Ctx) error {
	begin, end := parseTimeRange(c)
	data, err := s.logSvc.CountByDevice(context.Background(), begin, end)
	if err != nil {
		return c.JSON(model.Error(err.Error()))
	}
	return c.JSON(model.Success("按设备统计成功", data))
}

// Internal admin endpoints for the lightweight UI (raw JSON)
func (s *Server) handleAdminListDevices(c *fiber.Ctx) error {
	devices, err := s.deviceSvc.List(context.Background())
	if err != nil {
		return s.fail(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(devices)
}

func (s *Server) handleAdminGetDevice(c *fiber.Ctx) error {
	device, err := s.deviceSvc.Get(context.Background(), c.Params("token"))
	if err != nil {
		if err == storage.ErrNotFound {
			return s.fail(c, http.StatusNotFound, "device not found")
		}
		return s.fail(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(device)
}

func (s *Server) handleAdminUpsertDevice(c *fiber.Ctx) error {
	var req service.DeviceRequest
	if err := c.BodyParser(&req); err != nil {
		return s.fail(c, http.StatusBadRequest, err.Error())
	}
	device, err := s.deviceSvc.Upsert(context.Background(), req)
	if err != nil {
		return s.fail(c, http.StatusBadRequest, err.Error())
	}
	return c.JSON(device)
}

func (s *Server) handleAdminSummary(c *fiber.Ctx) error {
	ctx := context.Background()
	devices, err := s.deviceSvc.List(ctx)
	if err != nil {
		return s.fail(c, http.StatusInternalServerError, err.Error())
	}
	active := 0
	for _, d := range devices {
		if strings.EqualFold(d.Status, model.DeviceStatusActive) || d.Status == "" {
			active++
		}
	}
	logs, err := s.store.ListNoticeLogs(ctx)
	if err != nil {
		logs = nil
	}
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].CreatedAt.After(logs[j].CreatedAt)
	})
	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	todaySent := 0
	todaySuccess := 0
	for _, log := range logs {
		if !log.CreatedAt.Before(todayStart) {
			todaySent++
			if strings.EqualFold(log.Status, "SUCCESS") {
				todaySuccess++
			}
		} else {
			break
		}
	}
	recent := make([]fiber.Map, 0, 5)
	for i := 0; i < len(logs) && i < 5; i++ {
		recent = append(recent, fiber.Map{
			"title":     logs[i].Title,
			"group":     logs[i].Group,
			"status":    logs[i].Status,
			"deviceKey": maskKey(logs[i].DeviceKey),
			"time":      logs[i].CreatedAt.Local().Format("01-02 15:04"),
		})
	}
	status := "离线"
	if s.pingBark() {
		status = "在线"
	}
	return c.JSON(model.Success("ok", fiber.Map{
		"status":       status,
		"active":       active,
		"total":        len(devices),
		"todaySent":    todaySent,
		"todaySuccess": todaySuccess,
		"recentLogs":   recent,
	}))
}

func (s *Server) fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": message})
}

func (s *Server) serveFrontend() {
	dir := strings.TrimSpace(s.cfg.Frontend.Dir)
	if dir == "" {
		return
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	s.app.Static("/", dir, fiber.Static{
		Index:    "index.html",
		Compress: true,
	})
}

func decodePathSegment(value string) string {
	if value == "" {
		return value
	}
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func parseLogFilter(c *fiber.Ctx) model.NoticeLogFilter {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "10"))
	begin, end := parseTimeRange(c)
	return model.NoticeLogFilter{
		DeviceKey: c.Query("deviceKey"),
		Group:     c.Query("group"),
		Status:    c.Query("status"),
		BeginTime: begin,
		EndTime:   end,
		Page:      page,
		PageSize:  pageSize,
	}
}

func parseTimeRange(c *fiber.Ctx) (*time.Time, *time.Time) {
	begin := parseTime(c.Query("beginTime"))
	end := parseTime(c.Query("endTime"))
	return begin, end
}

func parseTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	return nil
}

func (s *Server) requireAuth(c *fiber.Ctx) error {
	if s.authSvc == nil || !s.authSvc.Enabled() {
		return c.Next()
	}
	token := extractBearerToken(c.Get("Authorization"))
	if token == "" {
		return c.Status(http.StatusUnauthorized).JSON(model.Error("未登录"))
	}
	claims, err := s.authSvc.Validate(token)
	if err != nil {
		return c.Status(http.StatusUnauthorized).JSON(model.Error("登录已失效"))
	}
	c.Locals("username", claims.Username)
	return c.Next()
}

func extractBearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func (s *Server) authSvcUsername() string {
	if s.authSvc == nil {
		return ""
	}
	return s.authSvc.Username()
}

func maskKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return value
	}
	return string(runes[:4]) + strings.Repeat("*", len(runes)-4)
}
