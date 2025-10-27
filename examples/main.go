package main

import (
	"fmt"
	"os"

	"github.com/dangdungcntt/go-blade"
)

func main() {
	eng := blade.New("examples/views")
	if err := eng.Load(); err != nil {
		panic(err)
	}
	data := map[string]interface{}{"Name": "John Doe"}
	if err := eng.Render(os.Stdout, "pages/home", data); err != nil {
		fmt.Println("render error:", err)
	}
}
