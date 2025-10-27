package main

import (
	"github.com/dangdungcntt/go-blade"
	"github.com/gin-gonic/gin"
)

func main() {
	bladeEngine := blade.NewEngine("examples/views")
	bladeEngine.FuncMap["hello"] = func(name string) string {
		return "Hello " + name
	}
	if err := bladeEngine.Load(); err != nil {
		panic(err)
	}

	ginEngine := gin.Default()
	ginEngine.HTMLRender = blade.NewHTMLRender(bladeEngine)

	ginEngine.GET("/", func(c *gin.Context) {
		data := map[string]any{"Name": "John Doe"}
		c.HTML(200, "pages/home", data)
	})
	ginEngine.GET("/about", func(c *gin.Context) {
		c.HTML(200, "pages/about", nil)
	})

	err := ginEngine.Run(":8080")
	if err != nil {
		panic(err)
	}
}
