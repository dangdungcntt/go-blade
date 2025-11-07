package main

import (
	"html/template"

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
	ginEngine.Use(func(c *gin.Context) {
		// For development, reload templates on each request.
		err := bladeEngine.Load()
		if err != nil {
			c.Status(500)
			c.String(500, err.Error())
			c.Abort()
			return
		}

		c.Next()
	})

	ginEngine.GET("/", func(c *gin.Context) {
		data := blade.NewDataWithFuncs(gin.H{
			"Name": "John Doe",
		}, template.FuncMap{
			"hello": func(name string) string {
				return "Hello " + name + " from override funcs"
			},
		})
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
