package template_test

import (
	"bytes"
	"fmt"
	"html/template"
	"testing"

	"github.com/stretchr/testify/require"
)

// makeLargeTemplate tạo một template có đủ kích thước/độ phức tạp
// để chi phí parse/clone/execute rõ rệt trong benchmark.
func makeLargeTemplate() string {
	// tạo 1 template với nhiều block / range / condition để nặng hơn
	var b bytes.Buffer
	b.WriteString(`{{define "row"}}<div class="row">{{.Index}}: {{.Text}}</div>{{end}}`)
	b.WriteString("\n<ul>\n")
	b.WriteString(`{{range $i, $it := .Items}}<li>{{template "row" (dict "Index" $i "Text" $it)}}</li>{{end}}`)
	b.WriteString("\n</ul>\n")
	// lặp thêm nhiều lần để tăng kích thước parse-tree
	for i := range 20 {
		b.WriteString("\n<!-- block " + template.HTMLEscaper(i) + " -->")
	}
	return b.String()
}

var (
	tplSource = makeLargeTemplate()
	funcs     = template.FuncMap{
		"dict": func(v ...any) map[string]any {
			dict := map[string]any{}
			lenv := len(v)
			for i := 0; i < lenv; i += 2 {
				key := fmt.Sprint(v[i])
				if i+1 >= lenv {
					dict[key] = ""
					continue
				}
				dict[key] = v[i+1]
			}
			return dict
		},
	}
)

type viewData struct {
	Items []string
}

func benchData() viewData {
	items := make([]string, 100)
	for i := range items {
		items[i] = "Item number " + template.HTMLEscaper(i)
	}
	return viewData{Items: items}
}

// 1) Reuse parsed template and Execute directly (concurrent-safe)
func Benchmark_Template_CachedExecute(b *testing.B) {
	t, err := template.New("big").Funcs(funcs).Parse(tplSource)
	require.NoError(b, err, "parse template failed")

	data := benchData()
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var buf bytes.Buffer
		for pb.Next() {
			buf.Reset()
			if err := t.Execute(&buf, data); err != nil {
				b.Fatalf("execute failed: %v", err)
			}
		}
	})
}

// 2) Clone the parsed template before Execute (clone per run)
func Benchmark_Template_CachedCloneExecute(b *testing.B) {
	t := template.Must(template.New("big").Funcs(funcs).Parse(tplSource))
	data := benchData()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var buf bytes.Buffer
		for pb.Next() {
			buf.Reset()
			tc, err := t.Clone()
			if err != nil {
				b.Fatalf("clone failed: %v", err)
			}
			if err := tc.Execute(&buf, data); err != nil {
				b.Fatalf("execute failed: %v", err)
			}
		}
	})
}

// 3) Parse template on every iteration (uncached parse)
func Benchmark_Template_ParseEachTime(b *testing.B) {
	data := benchData()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var buf bytes.Buffer
		for pb.Next() {
			buf.Reset()
			t, err := template.New("big").Funcs(funcs).Parse(tplSource)
			if err != nil {
				b.Fatalf("parse failed: %v", err)
			}
			if err := t.Execute(&buf, data); err != nil {
				b.Fatalf("execute failed: %v", err)
			}
		}
	})
}
