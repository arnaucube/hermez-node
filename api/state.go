package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *API) getState(c *gin.Context) {
	ni, err := a.h.GetNodeInfo()
	if err != nil {
		retBadReq(err, c)
		return
	}
	c.JSON(http.StatusOK, ni.StateAPI)
}
