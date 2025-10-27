# go-blade — Blade-like Templating for Go

**go-blade** is a lightweight templating preprocessor that brings **Laravel Blade-style syntax** to Go’s standard `html/template` engine.

It lets you use `@extends`, `@section`, `@yield`, and `@include` just like in Blade, while staying 100% compatible with Go’s native template execution.

---

## ✨ Features

- ✅ Familiar Blade-like syntax:
    - `@extends('layout')` — inherit layouts
    - `@section('content', 'optinal default content') ... @endsection` — define page sections
    - `@yield('content', 'optinal default content')` — insert dynamic sections in layout
    - `@include('partial')` — include reusable fragments
- ⚡ Powered by Go’s safe and fast `html/template`
- 🔁 Recursive layout inheritance (layout → page → partial)
- 🧩 Supports `.blade`, `.tmpl`, `.html` files
- 📁 Automatic recursive loading of templates from directory

---

## 📦 Installation

```bash
go get github.com/dangdungcntt/go-blade
```

---

## 🧠 How It Works

go-blade works as a **preprocessor** that converts Blade-like directives into standard Go templates.

Example transformation:

```gotemplate
@extends('layouts/base')

@section('title')
Home Page
@endsection

@section('content')
<h1>Welcome {{.Name}}</h1>
@endsection
```

Becomes:

```gotemplate
{{define "title"}}Home Page{{end}}
{{define "content"}}<h1>Welcome {{.Name}}</h1>{{end}}
<html>
<head>
  <title>{{template "title"}}</title>
</head>
<body>
  {{template "content"}}
</body>
</html>
```

---

## 🧱 Project Structure Example

```
views/
 ├── layouts/
 │    └── base.blade
 ├── partials/
 │    └── nav.blade
 └── pages/
      └── home.blade
```

**views/layouts/base.blade**
```html
<!doctype html>
<html>
<head>
  <title>@yield('title')</title>
</head>
<body>
  @include('partials/nav')
  <main>
    @yield('content')
  </main>
</body>
</html>
```

**views/pages/home.blade**
```blade
@extends('layouts/base')

@section('title') Home @endsection

@section('content')
<h1>Hello {{.Name}}</h1>
@endsection
```

---

## 🚀 Usage Example

```go
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
```

## 🧑‍💻 License

MIT © 2025
