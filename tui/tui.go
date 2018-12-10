package tui

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"

	tui "github.com/marcusolsson/tui-go"
	"github.com/samsarahq/taskrunner"
)

func Option(r *taskrunner.Runtime) {
	ui := New()
	r.OnStart(func(ctx context.Context, executor *taskrunner.Executor) error {
		ui.executor = executor
		ui.tabWidget.executor = executor
		ui.Run(ctx, executor)
		return nil
	})
	r.Subscribe(ui.Subscribe)
}

type uiTab struct {
	task  *taskrunner.Task
	label *Label
	view  *logList
}

type tabWidget struct {
	sync.RWMutex
	executor       *taskrunner.Executor
	tabIndexByTask map[*taskrunner.Task]int
	*tui.Box
	bar      *tui.Box
	tabs     []*uiTab
	selected int
}

func (t *tabWidget) tab(task *taskrunner.Task) *uiTab {
	index, ok := t.tabIndexByTask[task]
	if !ok {
		return nil
	}

	return t.tabs[index]
}

type UI struct {
	tui.UI
	executor  *taskrunner.Executor
	done      bool
	tabWidget *tabWidget
}

func clamp(n, min, max int) int {
	if n < min {
		return max
	}
	if n > max {
		return min
	}
	return n
}

type Label struct {
	*tui.Box
	Child *tui.Label
	Style string
}

func (l *Label) Draw(p *tui.Painter) {
	p.WithStyle(l.Style, l.Box.Draw)
}

func (l *Label) SetStyleName(style string) {
	l.Style = style
}

func newLabel(name string) *Label {
	label := tui.NewLabel(name)
	return &Label{
		Box: tui.NewHBox(
			tui.NewSpacer(),
			tui.NewPadder(1, 0, label),
		),
		Child: label,
	}
}

func (t *tabWidget) OnKeyEvent(ev tui.KeyEvent) {
	switch ev.Rune {
	case 'j':
		t.Next()
	case 'k':
		t.Previous()
	case 'r':
		go func(e *taskrunner.Executor) {
			e.Invalidate(t.Current().task, taskrunner.UserRestart{})
		}(t.executor)
	}
	switch ev.Key {
	case tui.KeyTab:
		t.Next()
	case 278:
		t.Previous()
	case tui.KeyCtrlU, tui.KeyDown:
		t.ScrollDown()
	case tui.KeyCtrlD, tui.KeyUp:
		t.ScrollUp()
	}
	t.Box.OnKeyEvent(ev)
}

func (t *tabWidget) style() {
	for i := 0; i < len(t.tabs); i++ {
		if i == t.selected {
			t.tabs[i].label.SetStyleName("tab-selected")
			continue
		}
		t.tabs[i].label.SetStyleName("tab")
	}
}

func (t *tabWidget) setView(view tui.Widget) {
	t.Box.Remove(1)
	t.Box.Append(view)
}

func (t *tabWidget) Select(selection int) {
	t.selected = clamp(selection, 0, len(t.tabs)-1)
	t.style()
	t.setView(t.Current().view)
}
func (t *tabWidget) Current() *uiTab { return t.tabs[t.selected] }
func (t *tabWidget) Next()           { t.Select(t.selected + 1) }
func (t *tabWidget) Previous()       { t.Select(t.selected - 1) }
func (t *tabWidget) ScrollUp() {
	t.Current().view.Scroll(0, +5)
}
func (t *tabWidget) ScrollDown() {
	t.Current().view.Scroll(0, -5)
}

func New() *UI {
	ui := &UI{tabWidget: newTabWidget()}
	var err error
	ui.UI, err = tui.New(ui.tabWidget)
	if err != nil {
		log.Fatal(err)
	}

	ui.SetKeybinding("Ctrl+c", func() { ui.Quit() })
	ui.SetKeybinding("q", func() { ui.Quit() })

	theme := tui.NewTheme()
	theme.SetStyle("tab", tui.Style{Reverse: tui.DecorationOff})
	theme.SetStyle("tab-selected", tui.Style{Reverse: tui.DecorationOn})
	ui.SetTheme(theme)

	return ui
}

func (ui *UI) Quit() {
	ui.done = true
	ui.UI.Quit()

	pid := os.Getpid()
	process, err := os.FindProcess(pid)
	if err != nil {
		log.Fatalln(err)
	}

	if err := process.Signal(syscall.SIGINT); err != nil {
		log.Fatalln(err)
	}
}

func (ui *UI) Run(ctx context.Context, executor *taskrunner.Executor) error {
	ui.buildTabList(executor)

	go func() {
		<-ctx.Done()
		ui.Quit()
	}()
	go func() {
		if err := ui.UI.Run(); err != nil {
			log.Fatal(err)
		}
	}()
	return nil
}

func (t *tabWidget) populate(task *taskrunner.Task) bool {
	t.Lock()
	defer t.Unlock()
	tab := &uiTab{
		label: newLabel(task.Name),
		view:  newLogView(),
		task:  task,
	}
	t.tabs = append(t.tabs, tab)
	t.bar.Append(tab.label)

	t.tabIndexByTask[task] = len(t.tabs) - 1
	t.style()
	return true
}

func (ui *UI) buildTabList(executor *taskrunner.Executor) {
	for _, handler := range executor.Tasks() {
		ui.tabWidget.populate(handler.Definition())
	}
	ui.tabWidget.bar.Append(tui.NewSpacer())
	ui.tabWidget.Select(0)
}

func (ui *UI) Subscribe(events <-chan taskrunner.ExecutorEvent) error {
	for event := range events {
		if ui.done {
			return nil
		}
		if event.TaskHandler() == nil {
			continue
		}

		tab := ui.tabWidget.tab(event.TaskHandler().Definition())
		if tab == nil {
			continue
		}

		switch ev := event.(type) {
		case *taskrunner.TaskLogEvent:
			ui.Update(func() {
				tab.view.Append(ev.Message)
			})
		case *taskrunner.TaskCompletedEvent:
			ui.Update(func() {
				tab.view.Append(fmt.Sprintf("Completed! (%d)", ev.Duration))
			})
		case *taskrunner.TaskFailedEvent:
			ui.Update(func() {
				tab.view.Append(fmt.Sprintf("Failed! (%v)", ev.Error))
			})
		case *taskrunner.TaskStoppedEvent:
			ui.Update(func() {
				tab.view.Append("Stopped!")
			})
		}
	}
	return nil
}

func newTabWidget() *tabWidget {
	w := &tabWidget{
		bar:            tui.NewVBox(),
		tabIndexByTask: make(map[*taskrunner.Task]int),
	}
	w.Box = tui.NewHBox(w.bar, tui.NewVBox())
	w.style()

	return w
}

type logList struct {
	sync.Mutex
	tui.Widget
	scroll *autoScrollArea
	list   *tui.Box
}

func (l *logList) Scroll(dx, dy int) {
	l.scroll.Scroll(dx, dy)
}

func (l *logList) Append(log string) {
	l.Lock()
	defer l.Unlock()
	log = strings.TrimSuffix(log, "\n")
	l.list.Append(tui.NewLabel(log))
}

func newLogView() *logList {
	history := tui.NewVBox()
	scroll := newAutoScrollArea(tui.NewPadder(1, 0, history))
	scroll.ScrollToBottom()
	box := tui.NewVBox(scroll)
	box.SetSizePolicy(tui.Expanding, tui.Expanding)
	return &logList{list: history, scroll: scroll, Widget: box}
}
