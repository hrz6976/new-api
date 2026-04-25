package common

import (
	"errors"
	"net/http/httptest"
	"testing"

	basecommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGenBaseRelayInfo_RemapsOpenWebUIUser(t *testing.T) {
	origEnabled := basecommon.OpenWebUIUserIntegrationEnabled
	origFunc := basecommon.OpenWebUIUserIntegrationFunc
	t.Cleanup(func() {
		basecommon.OpenWebUIUserIntegrationEnabled = origEnabled
		basecommon.OpenWebUIUserIntegrationFunc = origFunc
	})

	basecommon.OpenWebUIUserIntegrationEnabled = true
	basecommon.OpenWebUIUserIntegrationFunc = func(email string) (int, int, string, error) {
		require.Equal(t, "mapped@example.com", email)
		return 42, 1234, "vip", nil
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-OpenWebUI-User-Email", "mapped@example.com")
	c.Request = req

	basecommon.SetContextKey(c, constant.ContextKeyUserId, 7)
	basecommon.SetContextKey(c, constant.ContextKeyUsingGroup, "token-group")
	basecommon.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	basecommon.SetContextKey(c, constant.ContextKeyUserQuota, 99)
	basecommon.SetContextKey(c, constant.ContextKeyUserEmail, "token@example.com")

	info := genBaseRelayInfo(c, nil)
	require.True(t, info.IsPlayground)
	require.Equal(t, 42, info.UserId)
	require.Equal(t, "mapped@example.com", info.UserEmail)
	require.Equal(t, 1234, info.UserQuota)
	require.Equal(t, "vip", info.UserGroup)
	require.Equal(t, "token-group", info.UsingGroup)
}

func TestGenBaseRelayInfo_DoesNotRemapUnknownOpenWebUIUser(t *testing.T) {
	origEnabled := basecommon.OpenWebUIUserIntegrationEnabled
	origFunc := basecommon.OpenWebUIUserIntegrationFunc
	t.Cleanup(func() {
		basecommon.OpenWebUIUserIntegrationEnabled = origEnabled
		basecommon.OpenWebUIUserIntegrationFunc = origFunc
	})

	basecommon.OpenWebUIUserIntegrationEnabled = true
	basecommon.OpenWebUIUserIntegrationFunc = func(email string) (int, int, string, error) {
		require.Equal(t, "missing@example.com", email)
		return 0, 0, "", errors.New("not found")
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-OpenWebUI-User-Email", "missing@example.com")
	c.Request = req

	basecommon.SetContextKey(c, constant.ContextKeyUserId, 7)
	basecommon.SetContextKey(c, constant.ContextKeyUsingGroup, "token-group")
	basecommon.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	basecommon.SetContextKey(c, constant.ContextKeyUserQuota, 99)
	basecommon.SetContextKey(c, constant.ContextKeyUserEmail, "token@example.com")

	info := genBaseRelayInfo(c, nil)
	require.False(t, info.IsPlayground)
	require.Equal(t, 7, info.UserId)
	require.Equal(t, "token@example.com", info.UserEmail)
	require.Equal(t, 99, info.UserQuota)
	require.Equal(t, "default", info.UserGroup)
}
