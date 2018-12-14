package tui

import (
	"context"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"

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

type UI struct {
	tui.UI
	executor  *taskrunner.Executor
	done      bool
	tabWidget *tabWidget
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

func (ui *UI) buildTabList(executor *taskrunner.Executor) {
	for _, handler := range executor.Tasks() {
		ui.tabWidget.populate(handler.Definition())
	}
	ui.tabWidget.bar.Append(tui.NewSpacer())
	ui.tabWidget.Select(0)
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
		case *taskrunner.TaskInvalidatedEvent:
			ui.Update(func() {
				tab.view.Append(fmt.Sprintf("Invalidating for %d reasons:", len(ev.Reasons)))
				for _, reason := range ev.Reasons {
					tab.view.Append(fmt.Sprintf("- %s", reason.Description()))
				}
			})
		case *taskrunner.TaskCompletedEvent:
			ui.Update(func() {
				var msg string
				if ev.Duration == 0 {
					msg = "Completed"
				} else {
					msg = fmt.Sprintf("Completed (%0.2fs)", float64(ev.Duration)/float64(time.Second))
				}
				tab.view.Append(msg)
			})
		case *taskrunner.TaskFailedEvent:
			ui.Update(func() {
				tab.view.Append("Failed:")
				tab.view.Append(ev.Error.Error())
			})
		case *taskrunner.TaskDiagnosticEvent:
			ui.Update(func() {
				tab.view.Append("Warning:")
				tab.view.Append(ev.Error.Error())
			})
		case *taskrunner.TaskStoppedEvent:
			ui.Update(func() {
				tab.view.Append("Stopped!")
			})
		}
	}
	return nil
}
