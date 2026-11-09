package apigateway

import (
	"database/sql"
	"fmt"
	"net/http"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/gmicro"
	"im-server/services/apigateway/routers"
	apiservices "im-server/services/apigateway/services"

	"github.com/gin-gonic/gin"
)

type ApiGateway struct {
	httpServer *gin.Engine
}

func (ser *ApiGateway) RegisterActors(register gmicro.IActorRegister) {

}

func (ser *ApiGateway) Startup(args map[string]interface{}) {
	if configures.Config.GetApiPort() > 0 {
		ser.httpServer = gin.Default()
		ser.httpServer.GET("/", func(ctx *gin.Context) {
			ctx.JSON(http.StatusOK, "ok-ok")
		})
		ser.httpServer.HEAD("/", func(ctx *gin.Context) {
			ctx.JSON(http.StatusOK, nil)
		})
		var policyDB *sql.DB
		if db := dbcommons.GetDb(); db != nil {
			var err error
			policyDB, err = db.DB()
			if err != nil {
				policyDB = nil
			}
		}
		policy, err := apiservices.NewMindlinkHTTP(configures.Config.Mindlink, policyDB)
		if err != nil {
			fmt.Println("Mindlink API remains closed:", err)
		}
		routers.Route(ser.httpServer, "apigateway", policy)

		httpPort := configures.Config.GetApiPort()
		go ser.httpServer.Run(fmt.Sprintf(":%d", httpPort))
		fmt.Println("Startup apigateway with port:", httpPort)
	}
}

func (ser *ApiGateway) Shutdown(force bool) {

}
