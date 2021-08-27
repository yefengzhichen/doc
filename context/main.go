package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()
	router.GET("/info", func(c *gin.Context) {
		//time.Sleep(2 * time.Second)
		ctx := c.Request.Context()
		fmt.Println(ctx.Err())
		str := "test context"
		fmt.Println("send test")
		data := []byte(str)
		c.Writer.Write(data)
	})

	router.Run(":8000")
}
