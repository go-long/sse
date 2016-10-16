package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/itcomusic/sse"
	"github.com/nu7hatch/gouuid"
)

type Message struct {
	Message string `form:"message" json:"message" binding:"required"`
}

type Person struct {
	Name string `form:"name"`
}

type repository struct {
	message   map[string]string
	currentID int
	maxID     int
	sync.Mutex
}

func main() {
	rep := &repository{
		message: make(map[string]string),
		maxID:   10,
	}
	// Create sse
	serveSSE := sse.New(&sse.Config{
		Retry: time.Second * 3,
	})
	defer serveSSE.Close()
	serveSSE.HandlerConnectNotify(func(id interface{}) {
		serveSSE.SendEvent(&sse.EventExcept{
			CID: []interface{}{id},
			Data: &sse.DataEvent{
				Value: "Connect new user",
			},
		})
	})
	serveSSE.HandlerDisconnectNotify(func(id interface{}) {
		serveSSE.SendEvent(&sse.EventExcept{
			CID: []interface{}{id},
			Data: &sse.DataEvent{
				Value: "Disconnect user:(",
			},
		})
	})
	serveSSE.HandlerReconnectNotify(func(rec *sse.Reconnect) {
		serveSSE.SendEvent(&sse.EventOnly{
			CID: []interface{}{rec.CID},
			Data: &sse.DataEvent{
				Value: "Connection was restored",
			},
		})
		defer rec.StopRecovery()
	})

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.Static("files", "./files")
	router.LoadHTMLGlob("templates/*")
	// Users
	authorized := router.Group("/", gin.BasicAuth(gin.Accounts{
		"foo":    "bar",
		"austin": "1234",
	}))
	// Render main chat
	authorized.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", Person{
			Name: c.MustGet(gin.AuthUserKey).(string),
		})
	})
	// Send message
	authorized.POST("/message", func(c *gin.Context) {
		// Read body, data json
		var json Message
		if c.BindJSON(&json) == nil {
			// Get user basic auth
			madeMsg := "(" + c.MustGet(gin.AuthUserKey).(string) + ")" + json.Message
			serveSSE.SendEvent(&sse.Event{
				Data: &sse.DataEvent{
					Value: madeMsg,
				},
				ID: strconv.Itoa(rep.currentID),
			})
			log.Printf("Made message - %s", madeMsg)
			c.JSON(http.StatusOK, fmt.Sprintf("%s message`s was sent", c.MustGet(gin.AuthUserKey).(string)))
		} else {
			c.JSON(http.StatusInternalServerError, ":(")
		}
	})
	// Get sse resourse
	authorized.GET("/events/", func(c *gin.Context) {
		u4, _ := uuid.NewV4()
		serveSSE.HandlerHTTP(u4, c.Writer, c.Request)
	})
	router.Run(":8080")
}
