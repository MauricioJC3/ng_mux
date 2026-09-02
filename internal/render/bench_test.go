package render

import "testing"

// BenchmarkComposeFresh allocates a new frame every call (the pre-refactor
// behaviour: NewFrame per tick). BenchmarkComposeReuse ping-pongs one buffer,
// as session.frame now does. Compare allocs/op between the two.
func BenchmarkComposeFresh(b *testing.B) {
	panes, status, style := sampleScene()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Compose(21, 6, panes, status, style)
	}
}

func BenchmarkComposeReuse(b *testing.B) {
	panes, status, style := sampleScene()
	var f *Frame
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f = ComposeInto(f, 21, 6, panes, status, style)
	}
}

// BenchmarkPaintNoOp is the cost of diffing two identical frames — every tick
// where a client is up to date. It should emit almost nothing.
func BenchmarkPaintNoOp(b *testing.B) {
	panes, status, style := sampleScene()
	f := Compose(21, 6, panes, status, style)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Paint(f, f)
	}
}
