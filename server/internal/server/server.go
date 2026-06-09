package server

import (
	"dingtalk/internal/api"
	"dingtalk/internal/database"
	"dingtalk/internal/logger"
	"dingtalk/internal/service"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"
)

func New(db *gorm.DB, distFS embed.FS) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			logger.Info("%s %s %d %s", c.Request().Method, c.Request().RequestURI, c.Response().Status, time.Since(start))
			return err
		}
	})
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	conversationService := service.NewConversationService(db)
	userService := service.NewUserService(db)
	messageService := service.NewMessageService(db)

	conversationHandler := api.NewConversationHandler(conversationService)
	userHandler := api.NewUserHandler(userService)
	messageHandler := api.NewMessageHandler(messageService, userService, db)

	apiGroup := e.Group("/api")
	apiGroup.GET("/conversations/home", conversationHandler.GetConversationsHome)
	apiGroup.GET("/conversations", conversationHandler.GetConversations)
	apiGroup.GET("/conversations/:cid/messages", messageHandler.GetConversationMessages)
	apiGroup.GET("/conversations/:cid/messages/search", messageHandler.SearchConversationMessages)
	apiGroup.GET("/messages/search", messageHandler.SearchMessagesGlobal)
	apiGroup.GET("/users", userHandler.GetUsers)
	apiGroup.GET("/users/search", userHandler.SearchUsers)

	var currentUser database.CurrentUser
	if err := db.First(&currentUser).Error; err == nil {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			// 指向钉钉的 ImageFiles 目录
			staticPath := filepath.Join(homeDir, ".config/DingTalk", fmt.Sprintf("%d_v2", currentUser.ID), "ImageFiles")
			e.Static("/static", staticPath)
			// 指向钉钉的 resource_cache 目录（高质量图片）
			resourceCachePath := filepath.Join(homeDir, ".config/DingTalk", fmt.Sprintf("%d_v2", currentUser.ID), "resource_cache")
			e.Static("/cache", resourceCachePath)
		}
	}

	distSubFS, _ := fs.Sub(distFS, "dist")
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(distSubFS))))

	return e
}
