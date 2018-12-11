package tui

import (
	"image"

	tui "github.com/marcusolsson/tui-go"
)

var _ tui.Widget = &autoScrollArea{}

// autoScrollArea is a widget to fill out space.
type autoScrollArea struct {
	tui.WidgetBase

	Widget tui.Widget

	topLeft    image.Point
	autoscroll bool
}

// newAutoScrollArea returns a new ScrollArea.
func newAutoScrollArea(w tui.Widget) *autoScrollArea {
	return &autoScrollArea{
		Widget:     w,
		autoscroll: true,
	}
}

// MinSizeHint returns the minimum size the widget is allowed to be.
func (s *autoScrollArea) MinSizeHint() image.Point {
	return image.ZP
}

// SizeHint returns the size hint of the underlying widget.
func (s *autoScrollArea) SizeHint() image.Point {
	return image.Pt(15, 8)
}

// Scroll shifts the views over the content.
func (s *autoScrollArea) Scroll(dx, dy int) {
	s.topLeft.X += dx
	s.topLeft.Y += dy

	s.autoscroll = false

	// Cap at top
	if s.IsScrollTop() {
		s.ScrollToTop()
	}

	// Cap at bottom
	if s.IsScrollBottom() {
		s.autoscroll = true
		s.ScrollToBottom()
	}
}

func (s *autoScrollArea) IsScrollBottom() bool { return s.topLeft.Y > s.Widget.SizeHint().Y-s.Size().Y }
func (s *autoScrollArea) IsScrollTop() bool    { return s.topLeft.Y < 0 }
func (s *autoScrollArea) IsScrollFilled() bool { return s.Size().Y-s.Widget.SizeHint().Y < 0 }

// ScrollToBottom ensures the bottom-most part of the scroll area is visible.
func (s *autoScrollArea) ScrollToBottom() {
	s.topLeft.Y = s.Widget.SizeHint().Y - s.Size().Y
}

// ScrollToTop resets the vertical scroll position.
func (s *autoScrollArea) ScrollToTop() { s.topLeft.Y = 0 }

// Draw draws the scroll area.
func (s *autoScrollArea) Draw(p *tui.Painter) {
	p.Translate(-s.topLeft.X, -s.topLeft.Y)
	defer p.Restore()

	off := image.Point{s.topLeft.X, s.topLeft.Y}
	p.WithMask(image.Rectangle{Min: off, Max: s.Size().Add(off)}, func(p *tui.Painter) {
		s.Widget.Draw(p)
	})
}

// Resize resizes the scroll area and the underlying widget.
func (s *autoScrollArea) Resize(size image.Point) {
	s.Widget.Resize(s.Widget.SizeHint())
	s.WidgetBase.Resize(size)

	if s.autoscroll && s.IsScrollFilled() {
		s.ScrollToBottom()
	}
}
