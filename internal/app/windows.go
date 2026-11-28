package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
)

const (
	childWindowCornerRadius = 10

	addSubscriptionWidth     = 480
	addSubscriptionHeight    = 340
	renameSubscriptionWidth  = 440
	renameSubscriptionHeight = 230
	deleteSubscriptionWidth  = 440
	deleteSubscriptionHeight = 220
	newProfileWidth          = 460
	newProfileHeight         = 230
	routingEditorWidth       = 700
	routingEditorHeight      = 780
	textViewerWidth          = 780
	textViewerHeight         = 640
)

type childWindow string

const (
	winAddSubscription childWindow = "add-subscription"
	winRenameSub       childWindow = "rename-subscription"
	winDeleteSub       childWindow = "delete-subscription"
	winNewProfile      childWindow = "new-profile"
	winRouting         childWindow = "routing"
	winConfig          childWindow = "config"
	winSingBox         childWindow = "singbox"
	winLog             childWindow = "log"
)

type textViewer int

const (
	viewerConfig textViewer = iota
	viewerSingBox
	viewerLog
)

type textView struct {
	Title string
	Path  string
	Text  string
	Err   string
}

func (a *App) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
	for _, w := range a.children {
		w.Invalidate()
	}
}

func (a *App) focusChild(key childWindow) bool {
	w := a.children[key]
	if w == nil {
		return false
	}
	w.Show()
	w.Focus()
	return true
}

func (a *App) openChild(key childWindow, options mygo.WindowOptions, view func(*ui.Context)) *mygo.Window {
	if a.win == nil {
		return nil
	}
	if a.children == nil {
		a.children = make(map[childWindow]*mygo.Window)
	}
	if w := a.children[key]; w != nil {
		w.Show()
		w.Focus()
		return w
	}
	options.Parent = a.win
	options.Content = ui.View(view)
	options.Frameless = true
	options.CornerRadius = childWindowCornerRadius
	options.DisableResize = true
	options.DisableMaximize = true
	options.DisableFullScreen = true
	if a.win != nil && options.Width > 0 && options.Height > 0 {
		x, y := a.win.Position()
		width, height := a.win.Size()
		if options.Width < width {
			options.X = x + (width-options.Width)/2
		}
		if options.Height < height {
			options.Y = y + (height-options.Height)/2
		} else {
			options.Y = y
		}
	}
	if options.BackgroundColor == "" {
		options.BackgroundColor = windowBackground
	}
	w := mygo.NewWindow(options)
	a.children[key] = w
	w.OnClosed(func() {
		delete(a.children, key)
		a.childClosed(key)
	})
	return w
}

func (a *App) closeChild(key childWindow) {
	w := a.children[key]
	if w == nil {
		return
	}
	delete(a.children, key)
	w.Close()
}

func (a *App) closeChildren() {
	if len(a.children) == 0 {
		return
	}
	windows := make([]*mygo.Window, 0, len(a.children))
	for _, w := range a.children {
		windows = append(windows, w)
	}
	a.children = nil
	for _, w := range windows {
		w.Close()
	}
}

func (a *App) childClosed(key childWindow) {
	switch key {
	case winAddSubscription:
		a.adding = false
		a.addName = ""
		a.addURL = ""
		a.err = ""
	case winRenameSub:
		a.subRenaming = ""
	case winDeleteSub:
		a.subDeleting = ""
	case winNewProfile:
		a.profileCreating = false
	case winRouting:
		a.profileDeleteConfirm = false
	case winConfig:
		delete(a.viewers, viewerConfig)
	case winSingBox:
		delete(a.viewers, viewerSingBox)
	case winLog:
		delete(a.viewers, viewerLog)
	}
	a.invalidate()
}

func (a *App) openAddSubscription() {
	if a.focusChild(winAddSubscription) {
		return
	}
	a.adding = true
	a.addName = ""
	a.addURL = ""
	a.err = ""
	a.openChild(winAddSubscription, mygo.WindowOptions{
		Title:  "Новая подписка",
		Width:  addSubscriptionWidth,
		Height: addSubscriptionHeight,
		Modal:  true,
	}, a.addSubscriptionView)
	a.invalidate()
}

func (a *App) openRenameSubscription(id string) {
	if a.focusChild(winRenameSub) {
		return
	}
	sub := a.cfg.Subscription(id)
	if sub == nil {
		return
	}
	a.subRenaming = id
	a.subNameDraft = sub.Name
	a.err = ""
	a.openChild(winRenameSub, mygo.WindowOptions{
		Title:  "Переименовать подписку",
		Width:  renameSubscriptionWidth,
		Height: renameSubscriptionHeight,
		Modal:  true,
	}, a.renameSubscriptionView)
	a.invalidate()
}

func (a *App) openDeleteSubscription(id string) {
	if a.focusChild(winDeleteSub) {
		return
	}
	if a.cfg.Subscription(id) == nil {
		return
	}
	a.subDeleting = id
	a.err = ""
	a.openChild(winDeleteSub, mygo.WindowOptions{
		Title:  "Удалить подписку",
		Width:  deleteSubscriptionWidth,
		Height: deleteSubscriptionHeight,
		Modal:  true,
	}, a.deleteSubscriptionView)
	a.invalidate()
}

func (a *App) openNewProfile() {
	if a.focusChild(winNewProfile) {
		return
	}
	if len(a.cfg.RoutingProfiles) >= config.MaxRoutingProfiles {
		a.notice = fmt.Sprintf("Не больше %d профилей маршрутизации.", config.MaxRoutingProfiles)
		a.invalidate()
		return
	}
	a.profileCreating = true
	a.profileNameDraft = fmt.Sprintf("Профиль %d", len(a.cfg.RoutingProfiles)+1)
	a.err = ""
	a.openChild(winNewProfile, mygo.WindowOptions{
		Title:  "Новый профиль",
		Width:  newProfileWidth,
		Height: newProfileHeight,
		Modal:  true,
	}, a.newProfileView)
	a.invalidate()
}

func (a *App) openProfileEditor() {
	if a.focusChild(winRouting) {
		return
	}
	profile := a.cfg.ActiveRoutingProfile()
	if profile == nil {
		return
	}
	a.profileNameDraft = profile.Name
	a.profileDeleteConfirm = false
	a.openChild(winRouting, mygo.WindowOptions{
		Title:  "Профиль " + profile.Name,
		Width:  routingEditorWidth,
		Height: routingEditorHeight,
		Modal:  true,
	}, a.routingEditorView)
	a.invalidate()
}

func (a *App) openTextViewer(kind textViewer) {
	key := winConfig
	title := "config.json"
	switch kind {
	case viewerSingBox:
		key = winSingBox
		title = "Конфигурация sing-box"
	case viewerLog:
		key = winLog
		title = "Журнал qbox"
	}
	if a.focusChild(key) {
		return
	}
	view := a.loadTextViewer(kind)
	a.viewers[kind] = &view
	a.openChild(key, mygo.WindowOptions{
		Title:  title,
		Width:  textViewerWidth,
		Height: textViewerHeight,
	}, func(c *ui.Context) { a.textViewerView(c, kind) })
}

func (a *App) loadTextViewer(kind textViewer) textView {
	switch kind {
	case viewerConfig:
		path := a.deps.ConfigPath
		data, err := readTail(path, configViewerLimit, 0)
		view := textView{Title: "config.json", Path: path, Text: string(data)}
		if err != nil {
			view.Err = err.Error()
		}
		return view
	case viewerSingBox:
		data := a.deps.Engine.LastConfig()
		view := textView{Title: "Конфигурация sing-box"}
		if len(data) == 0 {
			view.Text = "Конфигурация ещё не собрана.\n\nПодключитесь или перезапустите ядро, чтобы увидеть применённый конфиг."
			return view
		}
		view.Text = string(data)
		return view
	case viewerLog:
		dir, err := config.LogDir()
		if err != nil {
			return textView{Title: "Журнал qbox", Err: err.Error()}
		}
		path := filepath.Join(dir, "qbox.log")
		data, err := readTail(path, logViewerTail, logViewerTail)
		view := textView{Title: "Журнал qbox", Path: path, Text: string(data)}
		if err != nil {
			view.Err = err.Error()
		}
		return view
	}
	return textView{}
}

func (a *App) refreshTextViewer(kind textViewer) {
	view := a.loadTextViewer(kind)
	a.viewers[kind] = &view
	a.invalidate()
}

// readTail reads at most limit bytes, keeping the last `keep` bytes of a
// large file. keep is ignored when zero.
func readTail(path string, limit, keep int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() <= limit {
		return io.ReadAll(io.LimitReader(file, limit+1))
	}
	if keep <= 0 {
		return io.ReadAll(io.LimitReader(file, limit))
	}
	if _, err := file.Seek(info.Size()-keep, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, keep+1))
	if err != nil {
		return nil, err
	}
	return []byte("…\n" + strings.TrimLeft(string(data), "\r\n")), nil
}
