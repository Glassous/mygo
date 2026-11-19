//go:build windows && (amd64 || arm64)

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	win "github.com/egoist/mygo/internal/windows"
	"github.com/egoist/mygo/ui"
)

// TestConsumeActivationClickInactiveInput checks wheel, hover, and file drops
// over an inactive window when ConsumeActivationClick is enabled on Windows:
// native UI and embedded WebViews keep all three while inactive, while a
// regular WebView2 page disables its child control until the window activates.
func TestConsumeActivationClickInactiveInput(t *testing.T) {
	file := filepath.Join(t.TempDir(), "drop.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("native", func(t *testing.T) {
		var frames, hovered, dropped atomic.Int32
		var scroll ui.ScrollState
		var scrollY atomic.Value
		scrollY.Store(float32(0))
		w := newWindow(t, mygo.WindowOptions{
			Width: 300, Height: 200, X: 40, Y: 40,
			ConsumeActivationClick: true,
			Content: ui.View(func(c *ui.Context) {
				frames.Add(1)
				ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
					b := ui.Box(c).Width(300).Height(600)
					if b.Hovered() {
						hovered.Store(1)
					}
					if files := b.DroppedFiles(); len(files) > 0 {
						dropped.Store(int32(len(files)))
					}
				})
				scrollY.Store(scroll.Y)
			}),
		})
		eventually(t, "the native frame built", func() bool { return frames.Load() > 0 })
		other := newWindow(t, mygo.WindowOptions{Width: 200, Height: 150, X: 400, Y: 40, Content: ui.View(func(c *ui.Context) { ui.Text(c, "Other") })})
		other.Focus()
		requireForeground(t, other)
		eventually(t, "the other window focused", other.IsFocused)

		mygo.RunOnMain(func() { win.TestInputMove(w.NativeHandle(), 100, 100) })
		eventually(t, "hover over inactive native UI", func() bool { return hovered.Load() == 1 })
		mygo.RunOnMain(func() { win.TestInputWheel(w.NativeHandle(), 100, 100) })
		eventually(t, "wheel scroll over inactive native UI", func() bool {
			return scrollY.Load().(float32) > 0
		})
		droppedFile, supported := dragFiles(w, 100, 100, []string{file})
		if !supported || !droppedFile {
			t.Fatalf("inactive native drop: supported=%v dropped=%v", supported, droppedFile)
		}
		eventually(t, "drop over inactive native UI", func() bool { return dropped.Load() == 1 })
		if w.IsFocused() {
			t.Fatal("hover, wheel, or drop activated the window")
		}
	})

	t.Run("embedded", func(t *testing.T) {
		var frames atomic.Int32
		var view *mygo.WebView
		w := newWindow(t, mygo.WindowOptions{
			Width: 300, Height: 200, X: 40, Y: 40,
			ConsumeActivationClick: true,
			Content: ui.View(func(c *ui.Context) {
				frames.Add(1)
				if view != nil {
					ui.WebView(c, view).Fill()
				}
			}),
		})
		v, err := w.NewWebView(mygo.WebViewOptions{})
		if err != nil {
			t.Fatal(err)
		}
		w.Update(func() {
			frames.Store(0)
			view = v
		})
		eventually(t, "the embedded frame built", func() bool { return frames.Load() > 0 })
		page := v.Page()
		page.LoadHTML(`<body style="margin:0;height:200vh" onmousemove="window.hovered=true" onwheel="window.wheeled=true" ondragover="event.preventDefault()" ondrop="event.preventDefault(); window.dropped=event.dataTransfer.files.length"></body>`, "")
		waitForPage(t, page, "document.body !== null")

		other := newWindow(t, mygo.WindowOptions{Width: 200, Height: 150, X: 400, Y: 40, Content: ui.View(func(c *ui.Context) { ui.Text(c, "Other") })})
		other.Focus()
		requireForeground(t, other)
		eventually(t, "the other window focused", other.IsFocused)

		mygo.RunOnMain(func() { win.TestInputMove(w.NativeHandle(), 100, 100) })
		waitForPage(t, page, "window.hovered === true")
		mygo.RunOnMain(func() { win.TestInputWheel(w.NativeHandle(), 100, 100) })
		waitForPage(t, page, "window.wheeled === true")
		ok, supported := dragFiles(w, 100, 100, []string{file})
		if !supported || !ok {
			t.Fatalf("inactive embedded drop: supported=%v ok=%v", supported, ok)
		}
		waitForPage(t, page, "window.dropped === 1")
		if w.IsFocused() {
			t.Fatal("hover, wheel, or drop activated the window")
		}
	})

	t.Run("page", func(t *testing.T) {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v", enabled), func(t *testing.T) {
				w := newWindow(t, mygo.WindowOptions{
					Width: 300, Height: 200, X: 40, Y: 40,
					ConsumeActivationClick: enabled,
				})
				page := w.Page()
				page.LoadHTML(`<body style="margin:0;height:200vh" onmousemove="window.hovered=true" onwheel="window.wheeled=true"></body>`, "")
				waitForPage(t, page, "document.body !== null")

				other := newWindow(t, mygo.WindowOptions{Width: 200, Height: 150, X: 400, Y: 40, Content: ui.View(func(c *ui.Context) { ui.Text(c, "Other") })})
				other.Focus()
				requireForeground(t, other)
				eventually(t, "the other window focused", other.IsFocused)

				var childEnabled, ok bool
				mygo.RunOnMain(func() { childEnabled, ok = win.TestWebViewChildEnabled(w.NativeHandle()) })
				if !ok {
					t.Fatal("WebView2 child window not found")
				}
				if childEnabled == enabled {
					t.Fatalf("inactive Chrome_WidgetWin_0 enabled = %v, want %v", childEnabled, !enabled)
				}
				if !enabled {
					mygo.RunOnMain(func() { win.TestInputMove(w.NativeHandle(), 100, 100) })
					waitForPage(t, page, "window.hovered === true")
					mygo.RunOnMain(func() { win.TestInputWheel(w.NativeHandle(), 100, 100) })
					waitForPage(t, page, "window.wheeled === true")
					return
				}
				// While inactive with ConsumeActivationClick on, disabled Chrome_WidgetWin_0
				// withholds hover and wheel; once activated, input resumes.
				mygo.RunOnMain(func() { win.TestInputMove(w.NativeHandle(), 100, 100) })
				mygo.RunOnMain(func() { win.TestInputWheel(w.NativeHandle(), 100, 100) })
				if got, err := mygo.EvalAs[bool](page, "Boolean(window.hovered || window.wheeled)"); err != nil || got {
					t.Fatalf("inactive page received hover/wheel = %v (err=%v), want false", got, err)
				}
				w.Focus()
				eventually(t, "the page window focused", w.IsFocused)
				mygo.RunOnMain(func() { childEnabled, ok = win.TestWebViewChildEnabled(w.NativeHandle()) })
				if !ok || !childEnabled {
					t.Fatalf("active Chrome_WidgetWin_0 enabled = %v (ok=%v), want true", childEnabled, ok)
				}
				mygo.RunOnMain(func() { win.TestInputMove(w.NativeHandle(), 100, 100) })
				waitForPage(t, page, "window.hovered === true")
				mygo.RunOnMain(func() { win.TestInputWheel(w.NativeHandle(), 100, 100) })
				waitForPage(t, page, "window.wheeled === true")
			})
		}
	})
}
