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

```html
@extends('layouts/base')

@section('title')
Home Page
@endsection

@section('content')
<h1>Welcome {{.Name}}</h1>
@endsection
```

Becomes:

```html
{{define "__section_title"}}Home Page{{end}}
{{define "__section_content"}}<h1>Welcome {{.Name}}</h1>{{end}}
<html>
<head>
  <title>{{ template "__section_title" .}}</title>
</head>
<body>
  {{ template "__section_content" . }}
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

### 1. Conditional sections and push stacks

Since go-blade is a preprocessor, it cannot handle conditional sections or push stacks.

The following code will not work:

```html
<!--layout.gohtml-->
<html>
    <body>
        @stack('content')
    </body>
</html>
```
    
```html
<!--page.gohtml-->
@extends('layout')

{{ if .Var }}
    @push('content')
        Content
    @endpush
{{ end }}
```
    
Output:
    
```html
{{ define "__stack_content" }}Content{{ end }}
<html>
    <body>
        {{ template "__stack_content" . }}
    </body>
</html>
```

→ When a page extends a layout, only the content inside `@push` or `@section` directives will be rendered.

### 2. `@include` with complex data

For simplicity, go-blade only supports passing a simple expression (without parentheses) to a partial template.

```html
@include('partial', dict "Field" .OptionalData) // Works
```

To pass complex data to a partial template, use a `with` block:
    
```html
{{ with slice .Items 0 (min (len .Items) 5) }}
    @include('partial', dict "Items" .)
{{ end }}
```
    
### 3. Pass content as the second argument of `@section`

Due to parsing limitations, the shorthand syntax `@section('name', ...)` only accepts a string as the second argument. 

For more complex cases, use the full block form

```html
@section('name')
...
@endsection`
``` 

## License

MIT © 2025
