package apigateway

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"im-server/commons/configures"
	"im-server/commons/dbcommons"
	"im-server/commons/gmicro"
	"im-server/services/apigateway/routers"
	apiservices "im-server/services/apigateway/services"

	"github.com/gin-gonic/gin"
)

type ApiGateway struct {
	httpServer  *gin.Engine
	inboxCancel context.CancelFunc
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
		if configures.Config.Mindlink.Enabled && err == nil {
			if worker, ok := policy.(interface {
				DrainOwnerInbox(context.Context, string) (string, int, error)
				DrainOwnerNotifications(context.Context, string, func(context.Context, string, string, int64) error) (string, error)
			}); ok {
				workerCtx, cancel := context.WithCancel(context.Background())
				ser.inboxCancel = cancel
				go func() {
					ticker := time.NewTicker(time.Second)
					defer ticker.Stop()
					after := ""
					notifyAfter := ""
					for {
						select {
						case <-workerCtx.Done():
							return
						case <-ticker.C:
							ctx, done := context.WithTimeout(workerCtx, 10*time.Second)
							next, _, err := worker.DrainOwnerInbox(ctx, after)
							notifyNext, notifyErr := worker.DrainOwnerNotifications(ctx, notifyAfter, apiservices.SignalMindlinkOwner)
							notifyAfter = notifyNext
							done()
							after = next
							if (err != nil || notifyErr != nil) && workerCtx.Err() == nil {
								fmt.Println("Mindlink owner inbox pass incomplete; pending entries retained")
							}
						}
					}
				}()
			}
		}
		routers.Route(ser.httpServer, "apigateway", policy)

		httpPort := configures.Config.GetApiPort()
		go ser.httpServer.Run(fmt.Sprintf(":%d", httpPort))
		fmt.Println("Startup apigateway with port:", httpPort)
	}
}

func (ser *ApiGateway) Shutdown(force bool) {
	if ser.inboxCancel != nil {
		ser.inboxCancel()
	}
}
