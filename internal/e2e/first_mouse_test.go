//go:build (darwin && !ios) || (windows && (amd64 || arm64))

package e2e

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Activation clicks must preserve a player's state when requested, for
// both kinds of content and for pages embedded in native UI.
func TestConsumeActivationClick(t *testing.T) {
	for _, kind := range []string{"page", "native", "embedded"} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", kind, enabled), func(t *testing.T) {
				var clicks, pressesAtSecondClick, frames atomic.Int32
				var view *mygo.WebView
				opts := mygo.WindowOptions{
					Width: 300, Height: 200, X: 40, Y: 40,
					ConsumeActivationClick: enabled,
				}
				if kind != "page" {
					opts.Content = ui.View(func(c *ui.Context) {
						frames.Add(1)
						if kind == "native" {
							ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
								if ui.Box(c).Grow(1).Clicked() {
									clicks.Add(1)
								}
								if ui.Box(c).Grow(1).Clicked() {
									pressesAtSecondClick.Store(clicks.Add(1))
								}
							})
						} else if view != nil {
							ui.WebView(c, view).Fill()
						}
					})
				}
				w := newWindow(t, opts)
				page := w.Page()
				if kind == "embedded" {
					v, err := w.NewWebView(mygo.WebViewOptions{})
					if err != nil {
						t.Fatal(err)
					}
					w.Update(func() {
						frames.Store(0)
						view = v
					})
					page = v.Page()
				}
				if kind != "page" {
					eventually(t, "the native frame built", func() bool { return frames.Load() > 0 })
				}
				if page != nil {
					page.LoadHTML(`<body style="margin:0;height:100vh" onmousedown="window.presses++; if (event.clientX >= 150) window.pressesAtSecondClick = window.presses"><script>window.presses=0</script>`, "")
					waitForPage(t, page, "window.presses === 0")
				}
				other := newWindow(t, mygo.WindowOptions{Width: 200, Height: 150, X: 400, Y: 40, Content: ui.View(func(c *ui.Context) { ui.Text(c, "Other") })})
				other.Focus()
				requireForeground(t, other)
				eventually(t, "the other window focused", other.IsFocused)
				checkActivationHit(t, w, kind, enabled)
				if !appClick(w, 100, 100) {
					t.Skip("click automation not available on this platform")
				}
				eventually(t, "the clicked window focused", w.IsFocused)
				// Use a different target for the second click. Its handler
				// records the count after both presses have been handled,
				// so a transient count of one cannot satisfy the assertion.
				appClick(w, 200, 100)
				want := int32(2)
				if enabled {
					want = 1
				}
				var got int32
				if page == nil {
					eventually(t, "the second native click handled", func() bool { return pressesAtSecondClick.Load() > 0 })
					got = pressesAtSecondClick.Load()
				} else {
					waitForPage(t, page, "Number.isInteger(window.pressesAtSecondClick)")
					n, err := mygo.EvalAs[int](page, "window.pressesAtSecondClick")
					if err != nil {
						t.Fatal(err)
					}
					got = int32(n)
				}
				if got != want {
					t.Fatalf("presses after the second click = %d, want %d", got, want)
				}
			})
		}
	}
}
