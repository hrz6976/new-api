package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

type openWebUIWebhookUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type openWebUIWebhookPayload struct {
	Action string `json:"action"`
	User   string `json:"user"`
}

func buildOpenWebUIUsername(rawName string, email string) string {
	name := strings.TrimSpace(rawName)
	if name == "" {
		parts := strings.SplitN(strings.TrimSpace(email), "@", 2)
		name = parts[0]
	}
	name = strings.ReplaceAll(name, " ", "_")
	if name == "" {
		name = "openwebui_user"
	}
	if len(name) > model.UserNameMaxLength {
		name = name[:model.UserNameMaxLength]
	}
	if ok, _ := model.CheckUserExistOrDeleted(name, ""); !ok {
		return name
	}
	suffix := "_" + common.GetRandomString(4)
	baseMax := model.UserNameMaxLength - len(suffix)
	if baseMax < 1 {
		baseMax = 1
	}
	if len(name) > baseMax {
		name = name[:baseMax]
	}
	return name + suffix
}

func OpenWebUIWebhook(c *gin.Context) {
	var payload openWebUIWebhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		logger.LogError(c, fmt.Sprintf("OpenWebUI webhook invalid payload: %v", err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if payload.Action != "signup" {
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	var user openWebUIWebhookUser
	if err := common.UnmarshalJsonStr(payload.User, &user); err != nil {
		logger.LogError(c, fmt.Sprintf("OpenWebUI webhook invalid user payload: %v", err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user payload"})
		return
	}

	email := strings.TrimSpace(user.Email)
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email is required"})
		return
	}

	if model.IsEmailAlreadyTaken(email) {
		c.JSON(http.StatusOK, gin.H{"status": "already_exists"})
		return
	}

	initialPassword := common.GetRandomString(10)
	localUser := model.User{
		Username: buildOpenWebUIUsername(user.Name, email),
		Email:    email,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Password: initialPassword,
	}
	if strings.TrimSpace(user.Name) != "" {
		localUser.DisplayName = strings.TrimSpace(user.Name)
		if len(localUser.DisplayName) > model.UserNameMaxLength {
			localUser.DisplayName = localUser.DisplayName[:model.UserNameMaxLength]
		}
	} else {
		localUser.DisplayName = localUser.Username
	}

	if err := localUser.Insert(0); err != nil {
		common.ApiError(c, err)
		return
	}

	link := fmt.Sprintf("%s/console/token", system_setting.ServerAddress)
	subject := fmt.Sprintf("欢迎使用%s", common.SystemName)
	content := fmt.Sprintf("<p>您好，欢迎使用%s！</p><p>您的账号信息如下：<br>账号：%s<br>初始密码：%s</p><p>请点击<a href='%s'>这里</a>添加您的API令牌。</p><p>若无法点击链接，请复制以下地址到浏览器访问：<br>%s</p>", common.SystemName, email, initialPassword, link, link)
	if err := common.SendEmail(subject, email, content); err != nil {
		logger.LogWarn(c, fmt.Sprintf("OpenWebUI signup email failed for %s: %v", email, err))
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
