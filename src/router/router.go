package router

import (
	ginlogger "github.com/FabienMht/ginslog/logger"
	ginrecovery "github.com/FabienMht/ginslog/recovery"
	"github.com/gin-gonic/gin"
	"log/slog"
)

func DefaultRouter() *gin.Engine {
	r := gin.Default()

	r.Use(ginlogger.New(slog.Default()))
	r.Use(ginrecovery.New(slog.Default()))

	return r
}
