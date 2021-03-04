package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *API) getState(c *gin.Context) {
	ni, err := a.h.GetNodeInfoAPI()
	if err != nil {
		retBadReq(err, c)
		return
	}
	c.JSON(http.StatusOK, ni.APIState)
}
