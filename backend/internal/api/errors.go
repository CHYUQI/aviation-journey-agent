package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 契约规定 HTTP 层错误统一为一行：{"error": "..."}，前端直接展示，不做分支。
// 分析过程中的失败不走这里，而是体现在快照的 status = failed 上。

func badRequest(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": msg})
}

func notFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "journey not found"})
}

func serverError(c *gin.Context, err error) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
