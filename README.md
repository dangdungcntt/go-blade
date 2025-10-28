# go-blade - Blade-like Templating for Go

**go-blade** is a lightweight templating preprocessor that brings **Laravel Blade-style syntax** to Go’s standard `html/template` engine.

It lets you use `@extends`, `@section`, `@yield`, and `@include` just like in Blade, while staying 100% compatible with Go’s native template execution.

## Features

- Familiar Blade-like syntax:
    - `@extends('layout')` - inherit layouts
    - `@section('name') ... @endsection` - define page sections
    - `@section('name', 'content')` - define page sections with default content
    - `@yield('section_name', 'optinal default content')` - insert dynamic sections in layout
    - `@include('partial', .OptionalData)` - include reusable fragments with optional data
    - `@stack('name')` - create a stack for dynamic push content
    - `@push('stack_name') ... @endpush` - push content to a stack
- Powered by Go’s safe and fast `html/template`
- Recursive layout inheritance (layout → page → partial)
- Default file extensions: `.gohtml`, `.blade`, `.tmpl`, `.html`
- Automatic recursive loading of templates from a directory
- Gin integration

## Installation

```bash
go get github.com/dangdungcntt/go-blade
```

## How It Works

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
{{define "__yield_title"}}Home Page{{end}}
{{define "__yield_content"}}<h1>Welcome {{.Name}}</h1>{{end}}
<html>
<head>
  <title>{{ template "__yield_title" .}}</title>
</head>
<body>
  {{ template "__yield_content" . }}
</body>
</html>
```

## Usage Example

```go
package main

import (
  "fmt"
  "os"
  
  "github.com/dangdungcntt/go-blade"
)

func main() {
	eng := blade.NewEngine("views")
	if err := eng.Load(); err != nil {
		panic(err)
	}
	data := map[string]interface{}{"Name": "John Doe"}
	if err := eng.Render(os.Stdout, "pages/home", data); err != nil {
		fmt.Println("render error:", err)
	}
}
```

## Limitations

Since go-blade is a preprocessor, it cannot handle conditional sections or push stacks. The following code will not work:

```gotemplate
<!--layout.gohtml-->
<html>
<body>
@stack('content')
</body>
</html>
```

```gotemplate
<!--page.gohtml-->
@extends('layout')
{{ if .Var }}
@push('content')
Content
@endpush
{{ end }}
```

Output:

```gotemplate
{{ define "__stack_content" }}Content{{ end }}
<html>
<body>
{{ template "__stack_content" . }}
</body>
</html>
```

When a page extends a layout, only the content inside `@push`, `@section` will be rendered. 

## License

MIT © 2025
