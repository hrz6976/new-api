package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOpenWebUIControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		model.DB = originalDB
		model.LOG_DB = originalLogDB
	})

	return db
}

func postOpenWebUIWebhook(t *testing.T, body any) (*httptest.ResponseRecorder, map[string]string) {
	t.Helper()

	bodyBytes, err := common.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/openwebui/webhook", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	OpenWebUIWebhook(c)

	var response map[string]string
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	return w, response
}

func makeOpenWebUIWebhookBody(t *testing.T, action string, user map[string]string) map[string]string {
	t.Helper()

	userBytes, err := common.Marshal(user)
	require.NoError(t, err)
	return map[string]string{
		"action": action,
		"user":   string(userBytes),
	}
}

func TestBuildOpenWebUIUsernameNormalizesAndFallsBack(t *testing.T) {
	setupOpenWebUIControllerTestDB(t)

	require.Equal(t, "Ada_Lovelace", buildOpenWebUIUsername(" Ada Lovelace ", "ada@example.com"))
	require.Equal(t, "sam", buildOpenWebUIUsername("", "sam@example.com"))
	require.Equal(t, "openwebui_user", buildOpenWebUIUsername("", "@example.com"))
}

func TestBuildOpenWebUIUsernameAddsSuffixForExistingUsername(t *testing.T) {
	db := setupOpenWebUIControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Username: "sam", Email: "existing@example.com"}).Error)

	username := buildOpenWebUIUsername("", "sam@example.com")

	require.NotEqual(t, "sam", username)
	require.True(t, strings.HasPrefix(username, "sam_"))
	require.LessOrEqual(t, len(username), model.UserNameMaxLength)
}

func TestIsDuplicateUsernameError(t *testing.T) {
	require.False(t, isDuplicateUsernameError(nil))
	require.True(t, isDuplicateUsernameError(fmt.Errorf("ERROR: duplicate key value violates unique constraint \"users_username_key\"")))
	require.True(t, isDuplicateUsernameError(fmt.Errorf("username duplicate")))
	require.False(t, isDuplicateUsernameError(fmt.Errorf("email duplicate")))
}

func TestOpenWebUIWebhookCreatesUserFromSignupPayload(t *testing.T) {
	db := setupOpenWebUIControllerTestDB(t)
	originalSystemName := common.SystemName
	originalServerAddress := system_setting.ServerAddress
	t.Cleanup(func() {
		common.SystemName = originalSystemName
		system_setting.ServerAddress = originalServerAddress
	})
	common.SystemName = "New API Test"
	system_setting.ServerAddress = "https://new-api.example.test"

	w, response := postOpenWebUIWebhook(t, makeOpenWebUIWebhookBody(t, "signup", map[string]string{
		"id":    "openwebui-user-id",
		"name":  " Ada Lovelace ",
		"email": " ada@example.com ",
	}))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "success", response["status"])

	var user model.User
	require.NoError(t, db.Where("email = ?", "ada@example.com").First(&user).Error)
	require.Equal(t, "Ada_Lovelace", user.Username)
	require.Equal(t, "Ada Lovelace", user.DisplayName)
	require.Equal(t, common.RoleCommonUser, user.Role)
	require.Equal(t, common.UserStatusEnabled, user.Status)
	require.NotEmpty(t, user.Password)
	require.NotEqual(t, "ada@example.com", user.Password)
}

func TestOpenWebUIWebhookReturnsAlreadyExistsForDuplicateEmail(t *testing.T) {
	db := setupOpenWebUIControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Username: "existing", Email: "dupe@example.com"}).Error)

	w, response := postOpenWebUIWebhook(t, makeOpenWebUIWebhookBody(t, "signup", map[string]string{
		"name":  "Duplicate User",
		"email": "dupe@example.com",
	}))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "already_exists", response["status"])

	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("email = ?", "dupe@example.com").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestOpenWebUIWebhookCreatesFallbackUsernameWithCollisionSuffix(t *testing.T) {
	db := setupOpenWebUIControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Username: "sam", Email: "existing@example.com"}).Error)

	w, response := postOpenWebUIWebhook(t, makeOpenWebUIWebhookBody(t, "signup", map[string]string{
		"email": "sam@example.com",
	}))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "success", response["status"])

	var user model.User
	require.NoError(t, db.Where("email = ?", "sam@example.com").First(&user).Error)
	require.True(t, strings.HasPrefix(user.Username, "sam_"))
	require.LessOrEqual(t, len(user.Username), model.UserNameMaxLength)
	require.Equal(t, user.Username, user.DisplayName)
}

func TestOpenWebUIWebhookIgnoresNonSignupAction(t *testing.T) {
	w, response := postOpenWebUIWebhook(t, makeOpenWebUIWebhookBody(t, "signin", map[string]string{
		"email": "ignored@example.com",
	}))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ignored", response["status"])
}

func TestOpenWebUIWebhookRejectsInvalidPayload(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/openwebui/webhook", strings.NewReader("{not-json"))
	c.Request.Header.Set("Content-Type", "application/json")

	OpenWebUIWebhook(c)

	var response map[string]string
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "invalid payload", response["error"])
}

func TestOpenWebUIWebhookRejectsInvalidUserPayload(t *testing.T) {
	w, response := postOpenWebUIWebhook(t, map[string]string{
		"action": "signup",
		"user":   "{not-json",
	})

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "invalid user payload", response["error"])
}

func TestOpenWebUIWebhookRequiresEmail(t *testing.T) {
	w, response := postOpenWebUIWebhook(t, makeOpenWebUIWebhookBody(t, "signup", map[string]string{
		"name": "No Email",
	}))

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "email is required", response["error"])
}
